package client

import (
	"context"
	"errors"
	"strings"
	"time"

	"slimebot/internal/domain"

	sdk "github.com/coder/acp-go-sdk"
)

type Turn struct {
	PlanMode   bool
	Content    func(sdk.PromptCapabilities) []sdk.ContentBlock
	Update     func(sdk.SessionUpdate) error
	Permission func(context.Context, sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error)
}

func (m *Manager) Prompt(ctx context.Context, session *domain.Session, agentID string, turn Turn) (sdk.StopReason, error) {
	cfg, err := m.config(ctx, agentID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSec)*time.Second)
	defer cancel()
	var stop sdk.StopReason
	err = m.withProcess(ctx, session, cfg, func(ctx context.Context, current *entry) error {
		p := current.proc
		if err := p.setMode(ctx, turn.PlanMode); err != nil {
			return err
		}
		result, err := runPrompt(ctx, current, turn)
		stop = result.StopReason
		return err
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		err = errors.New(redactedError(err, cfg))
	}
	return stop, err
}

func (p *process) setMode(ctx context.Context, plan bool) error {
	if p.modes == nil {
		return nil
	}
	var selected sdk.SessionModeId
	for _, mode := range p.modes.AvailableModes {
		name := strings.ToLower(string(mode.Id))
		if plan && name == "plan" {
			selected = mode.Id
			break
		}
		if !plan && strings.EqualFold(string(p.modes.CurrentModeId), "plan") && (name == "code" || name == "build" || name == "default") {
			selected = mode.Id
			break
		}
	}
	if selected == "" || p.modes.CurrentModeId == selected {
		return nil
	}
	if _, err := p.conn.SetSessionMode(ctx, sdk.SetSessionModeRequest{SessionId: p.remote, ModeId: selected}); err != nil {
		return err
	}
	p.modes.CurrentModeId = selected
	return nil
}

func runPrompt(ctx context.Context, current *entry, turn Turn) (sdk.PromptResponse, error) {
	p := current.proc
	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	callbackErrors := make(chan error, 1)
	current.peer.setTurn(&turnHandler{ctx: turnCtx, remote: p.remote, update: turn.Update, permission: turn.Permission,
		onError: func(err error) {
			select {
			case callbackErrors <- err:
			default:
			}
			cancel()
		},
	})
	defer current.peer.setTurn(nil)
	type promptResult struct {
		response sdk.PromptResponse
		err      error
	}
	result := make(chan promptResult, 1)
	rpcCtx, stopRPC := context.WithCancel(context.WithoutCancel(ctx))
	defer stopRPC()
	content := turn.Content(p.initialized.AgentCapabilities.PromptCapabilities)
	go func() {
		response, err := p.conn.Prompt(rpcCtx, sdk.PromptRequest{SessionId: p.remote, Prompt: content})
		result <- promptResult{response, err}
	}()
	select {
	case reply := <-result:
		select {
		case err := <-callbackErrors:
			return reply.response, err
		default:
		}
		if turnCtx.Err() != nil {
			return reply.response, turnCtx.Err()
		}
		if reply.err != nil {
			p.close()
		}
		return reply.response, reply.err
	case <-turnCtx.Done():
	}
	shutdown, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancelShutdown()
	_ = p.conn.Cancel(shutdown, sdk.CancelNotification{SessionId: p.remote})
	select {
	case <-result:
	case <-shutdown.Done():
		p.close()
	}
	select {
	case err := <-callbackErrors:
		return sdk.PromptResponse{}, err
	default:
		return sdk.PromptResponse{}, turnCtx.Err()
	}
}
