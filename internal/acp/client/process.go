package client

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"slimebot/internal/domain"
	configsvc "slimebot/internal/services/config"
	buildversion "slimebot/internal/version"

	sdk "github.com/coder/acp-go-sdk"
)

type process struct {
	closed        atomic.Bool
	conn          *sdk.ClientSideConnection
	cmd           *exec.Cmd
	stdin, stdout *os.File
	done          chan struct{}
	closeOnce     sync.Once
	initialized   sdk.InitializeResponse
	remote        sdk.SessionId
	modes         *sdk.SessionModeState
}

func startProcess(ctx context.Context, cfg domain.ACPConfig, cwd string, callbacks *peerClient) (*process, error) {
	args, env, err := configsvc.ACPProcessValues(cfg)
	if err != nil {
		return nil, err
	}
	command := cfg.Command
	if !filepath.IsAbs(command) && filepath.Base(command) != command {
		command = filepath.Join(cwd, command)
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("ACP executable: %w", err)
	}
	cmd, err := agentCommand(path, args)
	if err != nil {
		return nil, err
	}
	cmd.Dir = cwd
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 100 * time.Millisecond
	for _, item := range cmd.Environ() {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if upper == "JWT_SECRET" || strings.HasPrefix(upper, "SLIMEBOT_") {
			continue
		}
		cmd.Env = append(cmd.Env, item)
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	inChild, stdin, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer inChild.Close()
	stdout, outChild, err := os.Pipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	defer outChild.Close()
	cmd.Stdin, cmd.Stdout = inChild, outChild
	if err := ctx.Err(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("start ACP process: %w", err)
	}
	// Close the child-side parent copies before waiting for EOF.
	_ = inChild.Close()
	_ = outChild.Close()
	p := &process{cmd: cmd, stdin: stdin, stdout: stdout, done: make(chan struct{})}
	p.conn = sdk.NewClientSideConnection(callbacks, stdin, stdout)
	p.conn.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := p.conn.Initialize(initCtx, sdk.InitializeRequest{ProtocolVersion: sdk.ProtocolVersionNumber,
		ClientInfo: &sdk.Implementation{Name: "slimebot", Title: new("SlimeBot"), Version: buildversion.Info().Version},
	})
	if err == nil && response.ProtocolVersion != sdk.ProtocolVersionNumber {
		err = fmt.Errorf("unsupported ACP protocol version %d", response.ProtocolVersion)
	}
	if err != nil {
		p.close()
		return nil, fmt.Errorf("ACP initialize: %w", err)
	}
	p.initialized = response
	return p, nil
}

func (p *process) alive() bool {
	if p.closed.Load() {
		return false
	}
	select {
	case <-p.done:
		return false
	case <-p.conn.Done():
		return false
	default:
		return true
	}
}

func (p *process) close() {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		_ = p.stdin.Close()
		select {
		case <-p.done:
		case <-time.After(150 * time.Millisecond):
			killAgentProcess(p.cmd)
		}
		_ = p.stdout.Close()
	})
}
