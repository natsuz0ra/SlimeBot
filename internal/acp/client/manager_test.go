package client

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"

	sdk "github.com/coder/acp-go-sdk"
)

// helperAgent is the ACP agent served by the re-executed test binary.
type helperAgent struct {
	mu       sync.Mutex
	scenario string
	conn     *sdk.AgentSideConnection
}

func (a *helperAgent) connection() *sdk.AgentSideConnection {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.conn
}

func (a *helperAgent) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	if a.scenario == "no-init" {
		return sdk.InitializeResponse{}, sdk.NewInternalError("initialize refused")
	}
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersionNumber,
		AgentInfo:       &sdk.Implementation{Name: "helper-agent", Title: sdk.Ptr("Helper Agent"), Version: "0.1.0"},
		AgentCapabilities: sdk.AgentCapabilities{
			LoadSession:      true,
			PromptCapabilities: sdk.PromptCapabilities{Image: true, EmbeddedContext: true},
		},
	}, nil
}

func (a *helperAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "helper-session", Modes: helperModes()}, nil
}

func (a *helperAgent) LoadSession(context.Context, sdk.LoadSessionRequest) (sdk.LoadSessionResponse, error) {
	return sdk.LoadSessionResponse{Modes: helperModes()}, nil
}

func (a *helperAgent) SetSessionMode(context.Context, sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	return sdk.SetSessionModeResponse{}, nil
}

func (a *helperAgent) SetSessionConfigOption(context.Context, sdk.SetSessionConfigOptionRequest) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, nil
}

func (a *helperAgent) Prompt(ctx context.Context, request sdk.PromptRequest) (sdk.PromptResponse, error) {
	conn := a.connection()
	send := func(text string) error {
		return conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: request.SessionId, Update: sdk.UpdateAgentMessageText(text)})
	}
	switch a.scenario {
	case "chat":
		if err := send("hello from helper"); err != nil {
			return sdk.PromptResponse{}, err
		}
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
	case "permission":
		response, err := conn.RequestPermission(ctx, sdk.RequestPermissionRequest{
			SessionId: request.SessionId,
			ToolCall:  sdk.ToolCallUpdate{ToolCallId: "tool-1", Title: sdk.PatchSet("Run command")},
			Options: []sdk.PermissionOption{
				{OptionId: "allow", Name: "Allow", Kind: sdk.PermissionOptionKindAllowOnce},
				{OptionId: "reject", Name: "Reject", Kind: sdk.PermissionOptionKindRejectOnce},
			},
		})
		if err != nil {
			return sdk.PromptResponse{}, err
		}
		text := "permission:cancelled"
		if response.Outcome.Selected != nil {
			text = "permission:" + string(response.Outcome.Selected.OptionId)
		}
		if err := send(text); err != nil {
			return sdk.PromptResponse{}, err
		}
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
	case "env":
		if err := send("env:" + os.Getenv("JWT_SECRET") + "|" + os.Getenv("SLIMEBOT_TEST_SECRET") + "|" + os.Getenv("CFG_TOKEN")); err != nil {
			return sdk.PromptResponse{}, err
		}
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
	case "cancel":
		if err := send("waiting"); err != nil {
			return sdk.PromptResponse{}, err
		}
		<-ctx.Done()
		return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
	case "hang":
		<-ctx.Done()
		return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
	default:
		return sdk.PromptResponse{}, sdk.NewInternalError("unknown helper scenario")
	}
}

func helperModes() *sdk.SessionModeState {
	return &sdk.SessionModeState{
		CurrentModeId:  "code",
		AvailableModes: []sdk.SessionMode{{Id: "code", Name: "Code"}, {Id: "plan", Name: "Plan"}},
	}
}

func (a *helperAgent) Cancel(context.Context, sdk.CancelNotification) error { return nil }
func (a *helperAgent) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	return sdk.AuthenticateResponse{}, sdk.NewMethodNotFound("authenticate")
}
func (a *helperAgent) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, sdk.NewMethodNotFound("logout")
}
func (a *helperAgent) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, sdk.NewMethodNotFound("session/close")
}
func (a *helperAgent) DeleteSession(context.Context, sdk.DeleteSessionRequest) (sdk.DeleteSessionResponse, error) {
	return sdk.DeleteSessionResponse{}, sdk.NewMethodNotFound("session/delete")
}
func (a *helperAgent) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound("session/list")
}
func (a *helperAgent) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, sdk.NewMethodNotFound("session/resume")
}

// TestACPHelperAgent is not a real test: the manager re-executes the test binary
// with ACP_TEST_SCENARIO set, and this function then speaks ACP on stdout.
func TestACPHelperAgent(t *testing.T) {
	scenario := os.Getenv("ACP_TEST_SCENARIO")
	if scenario == "" {
		t.Skip("helper process only")
	}
	protocol := os.Stdout
	os.Stdout = os.Stderr
	agent := &helperAgent{scenario: scenario}
	conn := sdk.NewAgentSideConnection(agent, protocol, os.Stdin)
	agent.mu.Lock()
	agent.conn = conn
	agent.mu.Unlock()
	<-conn.Done()
	os.Exit(0)
}

type fakeStore struct {
	mu       sync.Mutex
	configs  map[string]domain.ACPConfig
	bindings map[string]struct {
		agentID *string
		remote  string
	}
}

func newFakeStore(configs ...domain.ACPConfig) *fakeStore {
	store := &fakeStore{configs: make(map[string]domain.ACPConfig), bindings: make(map[string]struct {
		agentID *string
		remote  string
	})}
	for _, config := range configs {
		store.configs[config.ID] = config
	}
	return store
}

func (s *fakeStore) GetACPConfig(_ context.Context, id string) (domain.ACPConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config, ok := s.configs[id]
	if !ok {
		return domain.ACPConfig{}, apperrors.ErrNotFound
	}
	return config, nil
}

func (s *fakeStore) SetSessionACP(_ context.Context, id string, agentID *string, remote string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[id] = struct {
		agentID *string
		remote  string
	}{agentID: agentID, remote: remote}
	return nil
}

func (s *fakeStore) binding(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.bindings[id]
	return value.remote, ok
}

func helperConfig(t *testing.T, scenario string, timeoutSec int) domain.ACPConfig {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	args, err := json.Marshal([]string{"-test.run=TestACPHelperAgent"})
	if err != nil {
		t.Fatalf("encode args: %v", err)
	}
	env, err := json.Marshal(map[string]string{
		"ACP_TEST_SCENARIO": scenario,
		"CFG_TOKEN":         "configured-secret",
	})
	if err != nil {
		t.Fatalf("encode env: %v", err)
	}
	return domain.ACPConfig{
		ID: "cfg-" + scenario, Name: "helper " + scenario, Command: executable,
		ArgsJSON: string(args), EnvJSON: string(env), TimeoutSec: timeoutSec,
		IsEnabled: true, UpdatedAt: time.Now(),
	}
}

func newManager(t *testing.T, store Store) *Manager {
	t.Helper()
	manager := NewManager(store)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Close(ctx)
	})
	return manager
}

func testSession(t *testing.T) *domain.Session {
	t.Helper()
	// The child process inherits this directory, so avoid t.TempDir: on Windows
	// TempDir cleanup fails while the spawned ACP process still holds the cwd.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	return &domain.Session{ID: "session-1", WorkingDirectory: cwd}
}

func updateText(update sdk.SessionUpdate) string {
	if update.AgentMessageChunk != nil && update.AgentMessageChunk.Content.Text != nil {
		return update.AgentMessageChunk.Content.Text.Text
	}
	return ""
}

func waitForText(t *testing.T, updates <-chan sdk.SessionUpdate, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case update := <-updates:
			if strings.Contains(updateText(update), want) {
				return
			}
		case <-deadline:
			t.Fatalf("did not receive update containing %q", want)
		}
	}
}

func TestManagerTestConnectionHandshake(t *testing.T) {
	config := helperConfig(t, "chat", 30)
	manager := newManager(t, newFakeStore(config))
	result, err := manager.TestConnection(context.Background(), config.ID)
	if err != nil {
		t.Fatalf("test connection: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok result, got %+v", result)
	}
	if result.ProtocolVersion != sdk.ProtocolVersionNumber {
		t.Fatalf("protocol version = %d", result.ProtocolVersion)
	}
	if result.AgentInfo == nil || result.AgentInfo.Name != "helper-agent" || result.AgentInfo.Title == nil || *result.AgentInfo.Title != "Helper Agent" {
		t.Fatalf("agent info = %+v", result.AgentInfo)
	}
	if result.Capabilities == nil || !result.Capabilities.LoadSession {
		t.Fatalf("capabilities = %+v", result.Capabilities)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("latency = %d", result.LatencyMs)
	}
}

func TestManagerTestConnectionFailureIsReportedNotReturned(t *testing.T) {
	config := helperConfig(t, "no-init", 30)
	manager := newManager(t, newFakeStore(config))
	result, err := manager.TestConnection(context.Background(), config.ID)
	if err != nil {
		t.Fatalf("test connection should report failures in the result: %v", err)
	}
	if result.OK || !strings.Contains(result.Error, "initialize") {
		t.Fatalf("unexpected failure result: %+v", result)
	}
}

func TestManagerPromptCreatesAndReusesRemoteSession(t *testing.T) {
	config := helperConfig(t, "chat", 30)
	store := newFakeStore(config)
	manager := newManager(t, store)
	session := testSession(t)

	updates := make(chan sdk.SessionUpdate, 8)
	var content []sdk.ContentBlock
	turn := Turn{
		Content: func(sdk.PromptCapabilities) []sdk.ContentBlock {
			content = []sdk.ContentBlock{sdk.TextBlock("hello")}
			return content
		},
		Update: func(update sdk.SessionUpdate) error {
			updates <- update
			return nil
		},
	}
	stop, err := manager.Prompt(context.Background(), session, config.ID, turn)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != sdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", stop)
	}
	if len(content) != 1 || content[0].Text == nil || content[0].Text.Text != "hello" {
		t.Fatalf("prompt content = %+v", content)
	}
	waitForText(t, updates, "hello from helper")
	remote, ok := store.binding(session.ID)
	if !ok || remote != "helper-session" {
		t.Fatalf("remote session binding = %q, ok=%v", remote, ok)
	}

	// The second turn reuses the same remote session instead of creating another one.
	stop, err = manager.Prompt(context.Background(), session, config.ID, Turn{
		Content: func(sdk.PromptCapabilities) []sdk.ContentBlock { return []sdk.ContentBlock{sdk.TextBlock("again")} },
		Update:  func(sdk.SessionUpdate) error { return nil },
	})
	if err != nil || stop != sdk.StopReasonEndTurn {
		t.Fatalf("second prompt stop=%q err=%v", stop, err)
	}
}

func TestManagerPromptBridgesPermissions(t *testing.T) {
	config := helperConfig(t, "permission", 30)
	manager := newManager(t, newFakeStore(config))
	session := testSession(t)
	updates := make(chan sdk.SessionUpdate, 8)
	var permissions atomic.Int32
	stop, err := manager.Prompt(context.Background(), session, config.ID, Turn{
		Content: func(sdk.PromptCapabilities) []sdk.ContentBlock { return []sdk.ContentBlock{sdk.TextBlock("run")} },
		Update:  func(update sdk.SessionUpdate) error { updates <- update; return nil },
		Permission: func(_ context.Context, request sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
			permissions.Add(1)
			if request.ToolCall.ToolCallId != "tool-1" {
				return acpCancelled(), nil
			}
			return selectAllow(request.Options), nil
		},
	})
	if err != nil || stop != sdk.StopReasonEndTurn {
		t.Fatalf("prompt stop=%q err=%v", stop, err)
	}
	if permissions.Load() != 1 {
		t.Fatalf("permission bridge calls = %d", permissions.Load())
	}
	waitForText(t, updates, "permission:allow")
}

func TestManagerPromptCancelSendsSessionCancel(t *testing.T) {
	config := helperConfig(t, "cancel", 30)
	manager := newManager(t, newFakeStore(config))
	session := testSession(t)
	updates := make(chan sdk.SessionUpdate, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		stop sdk.StopReason
		err  error
	}
	results := make(chan result, 1)
	go func() {
		stop, err := manager.Prompt(ctx, session, config.ID, Turn{
			Content: func(sdk.PromptCapabilities) []sdk.ContentBlock { return []sdk.ContentBlock{sdk.TextBlock("wait")} },
			Update:  func(update sdk.SessionUpdate) error { updates <- update; return nil },
		})
		results <- result{stop: stop, err: err}
	}()
	waitForText(t, updates, "waiting")
	started := time.Now()
	cancel()
	select {
	case reply := <-results:
		if !errors.Is(reply.err, context.Canceled) {
			t.Fatalf("cancel error = %v", reply.err)
		}
		if elapsed := time.Since(started); elapsed > 3*time.Second {
			t.Fatalf("cancel took %s", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not stop after cancellation")
	}
}

func TestManagerPromptTimesOut(t *testing.T) {
	config := helperConfig(t, "hang", 1)
	manager := newManager(t, newFakeStore(config))
	session := testSession(t)
	started := time.Now()
	_, err := manager.Prompt(context.Background(), session, config.ID, Turn{
		Content: func(sdk.PromptCapabilities) []sdk.ContentBlock { return []sdk.ContentBlock{sdk.TextBlock("wait")} },
		Update:  func(sdk.SessionUpdate) error { return nil },
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 900*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("timeout elapsed = %s", elapsed)
	}
}

func TestManagerPromptFiltersInheritedEnvironment(t *testing.T) {
	t.Setenv("JWT_SECRET", "leak-me")
	t.Setenv("SLIMEBOT_TEST_SECRET", "leak-me-too")
	config := helperConfig(t, "env", 30)
	manager := newManager(t, newFakeStore(config))
	updates := make(chan sdk.SessionUpdate, 8)
	_, err := manager.Prompt(context.Background(), testSession(t), config.ID, Turn{
		Content: func(sdk.PromptCapabilities) []sdk.ContentBlock { return []sdk.ContentBlock{sdk.TextBlock("env")} },
		Update:  func(update sdk.SessionUpdate) error { updates <- update; return nil },
	})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	select {
	case update := <-updates:
		text := updateText(update)
		if strings.Contains(text, "leak-me") {
			t.Fatalf("inherited secret leaked to ACP process: %q", text)
		}
		if !strings.Contains(text, "configured-secret") {
			t.Fatalf("configured env missing: %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no environment update")
	}
}

func TestManagerResetRebindsRemoteSession(t *testing.T) {
	config := helperConfig(t, "chat", 30)
	store := newFakeStore(config)
	manager := newManager(t, store)
	agentID := config.ID
	session := testSession(t)
	session.ID = "session-reset"
	session.ACPAgentID = &agentID
	if _, err := manager.Prompt(context.Background(), session, config.ID, Turn{
		Content: func(sdk.PromptCapabilities) []sdk.ContentBlock { return []sdk.ContentBlock{sdk.TextBlock("first")} },
		Update:  func(sdk.SessionUpdate) error { return nil },
	}); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if remote, _ := store.binding(session.ID); remote != "helper-session" {
		t.Fatalf("initial binding = %q", remote)
	}
	if err := manager.Reset(context.Background(), session); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if remote, _ := store.binding(session.ID); remote != "helper-session" {
		t.Fatalf("binding after reset = %q", remote)
	}
	bindings := 0
	store.mu.Lock()
	bindings = len(store.bindings)
	store.mu.Unlock()
	if bindings != 1 {
		t.Fatalf("unexpected binding count %d", bindings)
	}
}

func acpCancelled() sdk.RequestPermissionResponse {
	return sdk.RequestPermissionResponse{Outcome: sdk.RequestPermissionOutcome{Cancelled: &sdk.RequestPermissionOutcomeCancelled{Outcome: "cancelled"}}}
}

func selectAllow(options []sdk.PermissionOption) sdk.RequestPermissionResponse {
	for _, option := range options {
		if option.Kind == sdk.PermissionOptionKindAllowOnce || option.Kind == sdk.PermissionOptionKindAllowAlways {
			return sdk.RequestPermissionResponse{Outcome: sdk.RequestPermissionOutcome{Selected: &sdk.RequestPermissionOutcomeSelected{Outcome: "selected", OptionId: option.OptionId}}}
		}
	}
	return acpCancelled()
}
