package client

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	configsvc "slimebot/internal/services/config"
	sessionsvc "slimebot/internal/services/session"

	sdk "github.com/coder/acp-go-sdk"
)

const idleTimeout = 15 * time.Minute

type Store interface {
	GetACPConfig(context.Context, string) (domain.ACPConfig, error)
	SetSessionACP(context.Context, string, *string, string) error
}

type entry struct {
	configID      string
	configVersion time.Time
	cwd           string
	proc          *process
	peer          *peerClient
	cancel        context.CancelFunc
	lastUsed      time.Time
}

type Manager struct {
	store      Store
	mu         sync.Mutex
	entries    map[string]*entry
	lifecycle  context.Context
	stop       context.CancelFunc
	reaperDone chan struct{}
	operations sync.WaitGroup
}

func NewManager(store Store) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{store: store, entries: make(map[string]*entry), lifecycle: ctx, stop: cancel, reaperDone: make(chan struct{})}
	go m.reap()
	return m
}

func (m *Manager) config(ctx context.Context, id string) (domain.ACPConfig, error) {
	cfg, err := m.store.GetACPConfig(ctx, id)
	if err != nil {
		return domain.ACPConfig{}, fmt.Errorf("ACP agent configuration: %w", err)
	}
	if !cfg.IsEnabled {
		return domain.ACPConfig{}, fmt.Errorf("%w: ACP agent is disabled", apperrors.ErrInvalidInput)
	}
	return cfg, nil
}

func workingDirectory(cwd string, cfg domain.ACPConfig) (string, error) {
	if cwd == "" {
		cwd = cfg.DefaultCwd
	}
	if cwd == "" {
		var err error
		cwd, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return sessionsvc.ValidateWorkingDirectory(cwd)
}

func (m *Manager) operation(ctx context.Context) (context.Context, func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lifecycle.Err() != nil {
		return nil, nil, m.lifecycle.Err()
	}
	m.operations.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.lifecycle, cancel)
	return ctx, func() { stop(); cancel(); m.operations.Done() }, nil
}

func (m *Manager) withProcess(ctx context.Context, session *domain.Session, cfg domain.ACPConfig, run func(context.Context, *entry) error) error {
	ctx, finish, err := m.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	cwd, err := workingDirectory(session.WorkingDirectory, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.mu.Lock()
	if ctx.Err() != nil {
		m.mu.Unlock()
		return ctx.Err()
	}
	previous := m.entries[session.ID]
	if previous != nil && previous.cancel != nil {
		m.mu.Unlock()
		return errors.New("ACP session is already busy")
	}
	current := previous
	if current == nil || current.configID != cfg.ID || !current.configVersion.Equal(cfg.UpdatedAt) || current.cwd != cwd || !current.proc.alive() {
		current = &entry{configID: cfg.ID, configVersion: cfg.UpdatedAt, cwd: cwd, peer: &peerClient{}}
		m.entries[session.ID] = current
	}
	current.cancel = cancel
	m.mu.Unlock()
	if previous != nil && previous != current {
		previous.proc.close()
	}

	defer func() {
		m.mu.Lock()
		current.cancel = nil
		current.lastUsed = time.Now()
		closeProcess := m.entries[session.ID] != current || current.proc == nil || !current.proc.alive()
		if closeProcess && m.entries[session.ID] == current {
			delete(m.entries, session.ID)
		}
		m.mu.Unlock()
		if closeProcess && current.proc != nil {
			current.proc.close()
		}
	}()
	if current.proc == nil {
		p, err := startProcess(ctx, cfg, cwd, current.peer)
		if err != nil {
			return err
		}
		m.mu.Lock()
		current.proc = p
		valid := m.entries[session.ID] == current && ctx.Err() == nil
		m.mu.Unlock()
		if !valid {
			return context.Canceled
		}
		if err := m.attach(ctx, session, cfg.ID, current); err != nil {
			p.close()
			return err
		}
	}
	return run(ctx, current)
}

func (m *Manager) attach(ctx context.Context, session *domain.Session, configID string, current *entry) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p := current.proc
	if session.ACPAgentID != nil && *session.ACPAgentID == configID && session.ACPRemoteSessionID != "" && p.initialized.AgentCapabilities.LoadSession {
		id := sdk.SessionId(session.ACPRemoteSessionID)
		result, err := p.conn.LoadSession(ctx, sdk.LoadSessionRequest{Cwd: current.cwd, SessionId: id, McpServers: []sdk.McpServer{}})
		if err == nil {
			p.remote, p.modes = id, result.Modes
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	result, err := p.conn.NewSession(ctx, sdk.NewSessionRequest{Cwd: current.cwd, McpServers: []sdk.McpServer{}})
	if err != nil {
		return fmt.Errorf("ACP session/new: %w", err)
	}
	if result.SessionId == "" || len(result.SessionId) > 128 {
		return errors.New("ACP agent returned an invalid session ID")
	}
	if err := m.store.SetSessionACP(ctx, session.ID, &configID, string(result.SessionId)); err != nil {
		return err
	}
	p.remote, p.modes = result.SessionId, result.Modes
	return nil
}

func (m *Manager) Reset(ctx context.Context, session *domain.Session) error {
	if session.ACPAgentID == nil {
		return apperrors.ErrNotFound
	}
	cfg, err := m.config(ctx, *session.ACPAgentID)
	if err != nil {
		return err
	}
	m.ForgetSession(session.ID)
	if err := m.store.SetSessionACP(ctx, session.ID, session.ACPAgentID, ""); err != nil {
		return err
	}
	cleared := *session
	cleared.ACPRemoteSessionID = ""
	return m.withProcess(ctx, &cleared, cfg, func(context.Context, *entry) error { return nil })
}

func (m *Manager) ForgetSession(id string) {
	m.evict(func(sessionID string, _ *entry) bool { return sessionID == id })
}

func (m *Manager) ClearBinding(ctx context.Context, id string, agentID *string) error {
	if err := m.store.SetSessionACP(ctx, id, agentID, ""); err != nil {
		return err
	}
	m.ForgetSession(id)
	return nil
}

func (m *Manager) ForgetConfig(id string) {
	m.evict(func(_ string, value *entry) bool { return value.configID == id })
}

func (m *Manager) evict(shouldRemove func(string, *entry) bool) {
	m.mu.Lock()
	var close []*process
	for id, value := range m.entries {
		if !shouldRemove(id, value) {
			continue
		}
		delete(m.entries, id)
		if value.cancel != nil {
			value.cancel()
		}
		if value.proc != nil {
			close = append(close, value.proc)
		}
	}
	m.mu.Unlock()
	var waiting sync.WaitGroup
	for _, p := range close {
		waiting.Go(p.close)
	}
	waiting.Wait()
}

func (m *Manager) reap() {
	defer close(m.reaperDone)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.lifecycle.Done():
			return
		case now := <-ticker.C:
			m.evict(func(_ string, value *entry) bool {
				return value.cancel == nil && (!value.proc.alive() || now.Sub(value.lastUsed) >= idleTimeout)
			})
		}
	}
}

func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	m.stop()
	m.mu.Unlock()
	m.evict(func(string, *entry) bool { return true })
	done := make(chan struct{})
	go func() { <-m.reaperDone; m.operations.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

type TestResult struct {
	OK              bool                   `json:"ok"`
	ProtocolVersion sdk.ProtocolVersion    `json:"protocolVersion,omitempty"`
	AgentInfo       *sdk.Implementation    `json:"agentInfo,omitempty"`
	Capabilities    *sdk.AgentCapabilities `json:"capabilities,omitempty"`
	LatencyMs       int64                  `json:"latencyMs"`
	Error           string                 `json:"error,omitempty"`
}

func (m *Manager) TestConnection(ctx context.Context, id string) (TestResult, error) {
	ctx, finish, err := m.operation(ctx)
	if err != nil {
		return TestResult{}, err
	}
	defer finish()
	cfg, err := m.store.GetACPConfig(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	start := time.Now()
	cwd, err := workingDirectory("", cfg)
	if err == nil {
		p, startErr := startProcess(ctx, cfg, cwd, &peerClient{})
		if startErr == nil {
			defer p.close()
			return TestResult{OK: true, ProtocolVersion: p.initialized.ProtocolVersion, AgentInfo: p.initialized.AgentInfo,
				Capabilities: &p.initialized.AgentCapabilities, LatencyMs: time.Since(start).Milliseconds()}, nil
		}
		err = startErr
	}
	return TestResult{Error: redactedError(err, cfg), LatencyMs: time.Since(start).Milliseconds()}, nil
}

func redactedError(err error, cfg domain.ACPConfig) string {
	message := err.Error()
	_, env, _ := configsvc.ACPProcessValues(cfg)
	for _, value := range env {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return message
}
