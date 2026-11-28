package client

import (
	"context"
	"sync"

	acpbridge "slimebot/internal/acp"

	sdk "github.com/coder/acp-go-sdk"
)

type turnHandler struct {
	ctx        context.Context
	remote     sdk.SessionId
	update     func(sdk.SessionUpdate) error
	permission func(context.Context, sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error)
	onError    func(error)
}

type peerClient struct {
	mu   sync.Mutex
	turn *turnHandler
}

func (c *peerClient) setTurn(turn *turnHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn = turn
}

func (c *peerClient) currentTurn(remote sdk.SessionId) *turnHandler {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.turn == nil || c.turn.remote != remote || c.turn.ctx.Err() != nil {
		return nil
	}
	return c.turn
}

func (c *peerClient) SessionUpdate(_ context.Context, request sdk.SessionNotification) error {
	turn := c.currentTurn(request.SessionId)
	if turn == nil || turn.update == nil {
		return nil
	}
	if err := turn.update(request.Update); err != nil {
		turn.onError(err)
		return err
	}
	return nil
}

func (c *peerClient) RequestPermission(ctx context.Context, request sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	turn := c.currentTurn(request.SessionId)
	if turn == nil || turn.permission == nil {
		return acpbridge.CancelledPermission(), nil
	}
	waitCtx, cancel := context.WithCancel(turn.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer cancel()
	defer stop()
	resp, err := turn.permission(waitCtx, request)
	if waitCtx.Err() != nil {
		return acpbridge.CancelledPermission(), nil
	}
	if err != nil {
		turn.onError(err)
	}
	return resp, err
}

// SlimeBot advertises no ACP file, terminal, or elicitation capabilities.
func (*peerClient) CompleteElicitation(context.Context, sdk.CompleteElicitationNotification) error {
	return nil
}
func (*peerClient) CreateElicitation(context.Context, sdk.CreateElicitationRequest) (sdk.CreateElicitationResponse, error) {
	return sdk.CreateElicitationResponse{}, sdk.NewMethodNotFound("session/create_elicitation")
}
func (*peerClient) ReadTextFile(context.Context, sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	return sdk.ReadTextFileResponse{}, sdk.NewMethodNotFound("fs/read_text_file")
}
func (*peerClient) WriteTextFile(context.Context, sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	return sdk.WriteTextFileResponse{}, sdk.NewMethodNotFound("fs/write_text_file")
}
func (*peerClient) CreateTerminal(context.Context, sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	return sdk.CreateTerminalResponse{}, sdk.NewMethodNotFound("terminal/create")
}
func (*peerClient) KillTerminal(context.Context, sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	return sdk.KillTerminalResponse{}, sdk.NewMethodNotFound("terminal/kill")
}
func (*peerClient) TerminalOutput(context.Context, sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	return sdk.TerminalOutputResponse{}, sdk.NewMethodNotFound("terminal/output")
}
func (*peerClient) ReleaseTerminal(context.Context, sdk.ReleaseTerminalRequest) (sdk.ReleaseTerminalResponse, error) {
	return sdk.ReleaseTerminalResponse{}, sdk.NewMethodNotFound("terminal/release")
}
func (*peerClient) WaitForTerminalExit(context.Context, sdk.WaitForTerminalExitRequest) (sdk.WaitForTerminalExitResponse, error) {
	return sdk.WaitForTerminalExitResponse{}, sdk.NewMethodNotFound("terminal/wait_for_exit")
}
