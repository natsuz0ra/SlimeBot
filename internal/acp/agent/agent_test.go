package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	acpbridge "slimebot/internal/acp"
	acpagent "slimebot/internal/acp/agent"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"

	sdk "github.com/coder/acp-go-sdk"
)

// transcript records every byte the agent writes to its protocol output.
type transcript struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (r *transcript) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data.Write(p)
}

func (r *transcript) lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw := strings.Split(r.data.String(), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// assertJSONRPC fails when the agent emitted anything that is not a JSON-RPC 2.0 message.
func (r *transcript) assertJSONRPC(t *testing.T) {
	t.Helper()
	lines := r.lines()
	if len(lines) == 0 {
		t.Fatal("expected the agent to write protocol output")
	}
	for index, line := range lines {
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("stdout line %d is not JSON: %q", index, line)
		}
		if message["jsonrpc"] != "2.0" {
			t.Fatalf("stdout line %d is not a JSON-RPC 2.0 message: %q", index, line)
		}
		if message["id"] == nil && message["method"] == nil {
			t.Fatalf("stdout line %d is neither a request/response nor a notification: %q", index, line)
		}
	}
}

type recordingClient struct {
	mu                 sync.Mutex
	updates            []sdk.SessionUpdate
	permissionRequests []sdk.RequestPermissionRequest
	permission         func(sdk.RequestPermissionRequest) sdk.RequestPermissionResponse
}

func (c *recordingClient) SessionUpdate(_ context.Context, notification sdk.SessionNotification) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, notification.Update)
	return nil
}

func (c *recordingClient) RequestPermission(_ context.Context, request sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	c.mu.Lock()
	c.permissionRequests = append(c.permissionRequests, request)
	handler := c.permission
	c.mu.Unlock()
	if handler != nil {
		return handler(request), nil
	}
	return acpbridge.SelectPermission(request.Options, true), nil
}

func (c *recordingClient) snapshot() ([]sdk.SessionUpdate, []sdk.RequestPermissionRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sdk.SessionUpdate(nil), c.updates...), append([]sdk.RequestPermissionRequest(nil), c.permissionRequests...)
}

func (c *recordingClient) CompleteElicitation(context.Context, sdk.CompleteElicitationNotification) error {
	return nil
}
func (c *recordingClient) CreateElicitation(context.Context, sdk.CreateElicitationRequest) (sdk.CreateElicitationResponse, error) {
	return sdk.CreateElicitationResponse{}, sdk.NewMethodNotFound("session/create_elicitation")
}
func (c *recordingClient) ReadTextFile(context.Context, sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	return sdk.ReadTextFileResponse{}, sdk.NewMethodNotFound("fs/read_text_file")
}
func (c *recordingClient) WriteTextFile(context.Context, sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	return sdk.WriteTextFileResponse{}, sdk.NewMethodNotFound("fs/write_text_file")
}
func (c *recordingClient) CreateTerminal(context.Context, sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	return sdk.CreateTerminalResponse{}, sdk.NewMethodNotFound("terminal/create")
}
func (c *recordingClient) KillTerminal(context.Context, sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	return sdk.KillTerminalResponse{}, sdk.NewMethodNotFound("terminal/kill")
}
func (c *recordingClient) TerminalOutput(context.Context, sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	return sdk.TerminalOutputResponse{}, sdk.NewMethodNotFound("terminal/output")
}
func (c *recordingClient) ReleaseTerminal(context.Context, sdk.ReleaseTerminalRequest) (sdk.ReleaseTerminalResponse, error) {
	return sdk.ReleaseTerminalResponse{}, sdk.NewMethodNotFound("terminal/release")
}
func (c *recordingClient) WaitForTerminalExit(context.Context, sdk.WaitForTerminalExitRequest) (sdk.WaitForTerminalExitResponse, error) {
	return sdk.WaitForTerminalExitResponse{}, sdk.NewMethodNotFound("terminal/wait_for_exit")
}

type chatCall struct {
	sessionID, requestID, content, displayContent, modelID string
	attachmentIDs                                          []string
	thinkingLevel, subagentModelID, approvalMode           string
	planMode                                               bool
}

type fakeChat struct {
	mu      sync.Mutex
	calls   []chatCall
	handler func(context.Context, chatsvc.AgentCallbacks) error
	result  *chatsvc.ChatStreamResult
}

func (f *fakeChat) HandleChatStream(ctx context.Context, sessionID, requestID, content, displayContent, modelID string,
	attachmentIDs []string, thinkingLevel string, planMode bool, subagentModelID, approvalModeOverride string,
	callbacks chatsvc.AgentCallbacks) (*chatsvc.ChatStreamResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, chatCall{
		sessionID: sessionID, requestID: requestID, content: content, displayContent: displayContent,
		modelID: modelID, attachmentIDs: attachmentIDs, thinkingLevel: thinkingLevel, planMode: planMode,
		subagentModelID: subagentModelID, approvalMode: approvalModeOverride,
	})
	handler := f.handler
	result := f.result
	f.mu.Unlock()
	if handler != nil {
		if err := handler(ctx, callbacks); err != nil {
			return nil, err
		}
	}
	if result != nil {
		return result, nil
	}
	return &chatsvc.ChatStreamResult{}, nil
}

func (f *fakeChat) lastCall(t *testing.T) chatCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("expected a chat turn")
	}
	return f.calls[len(f.calls)-1]
}

type fakeSessions struct {
	mu       sync.Mutex
	created  []string
	snapshot domain.SessionExportSnapshot
	replayID string
}

func (f *fakeSessions) Create(_ context.Context, name string, workingDirectory ...string) (*domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, name)
	cwd := ""
	if len(workingDirectory) > 0 {
		cwd = workingDirectory[0]
	}
	return &domain.Session{ID: fmt.Sprintf("session-%d", len(f.created)), Name: name, WorkingDirectory: cwd}, nil
}

func (f *fakeSessions) ReplaySession(_ context.Context, id string) (domain.SessionExportSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replayID = id
	return f.snapshot, nil
}

type fakeModels struct{ models []domain.LLMConfig }

func (f *fakeModels) List(context.Context) ([]domain.LLMConfig, error) { return f.models, nil }

type fakeUploads struct {
	mu         sync.Mutex
	registered int
	consumed   []string
	cleaned    bool
}

func (f *fakeUploads) RegisterLocalFiles(_ string, files []chatsvc.LocalAttachmentFile) ([]chatsvc.UploadedAttachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered += len(files)
	items := make([]chatsvc.UploadedAttachment, 0, len(files))
	for index := range files {
		items = append(items, chatsvc.UploadedAttachment{ID: fmt.Sprintf("attachment-%d", index+1), Name: files[index].Name, MimeType: files[index].MimeType, SizeBytes: int64(len(files[index].Data))})
	}
	return items, nil
}

func (f *fakeUploads) Consume(_ string, ids []string) ([]chatsvc.UploadedAttachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.consumed = append(f.consumed, ids...)
	return nil, nil
}

func (f *fakeUploads) Cleanup([]chatsvc.UploadedAttachment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleaned = true
}

type harness struct {
	t          *testing.T
	client     *sdk.ClientSideConnection
	peer       *recordingClient
	output     *transcript
	ctx        context.Context
	cancel     context.CancelFunc
	agentDone  chan struct{}
	closeToSrv *io.PipeWriter
	closeFrom  *io.PipeReader
}

func newHarness(t *testing.T, options acpagent.Options) *harness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	toAgentReader, toAgentWriter := io.Pipe()
	fromAgentReader, fromAgentWriter := io.Pipe()
	output := &transcript{}
	peer := &recordingClient{}
	h := &harness{t: t, peer: peer, output: output, ctx: ctx, cancel: cancel, agentDone: make(chan struct{}), closeToSrv: toAgentWriter, closeFrom: fromAgentReader}
	go func() {
		defer close(h.agentDone)
		acpagent.Serve(ctx, toAgentReader, io.MultiWriter(fromAgentWriter, output), options, nil)
	}()
	h.client = sdk.NewClientSideConnection(peer, toAgentWriter, fromAgentReader)
	t.Cleanup(func() {
		cancel()
		_ = toAgentWriter.Close()
		_ = fromAgentReader.Close()
		select {
		case <-h.agentDone:
		case <-time.After(3 * time.Second):
			t.Error("agent connection did not stop")
		}
	})
	return h
}

func (h *harness) initialize() sdk.InitializeResponse {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := h.client.Initialize(ctx, sdk.InitializeRequest{
		ProtocolVersion: sdk.ProtocolVersionNumber,
		ClientInfo:      &sdk.Implementation{Name: "test-client", Title: sdk.Ptr("Test Client"), Version: "1.0.0"},
	})
	if err != nil {
		h.t.Fatalf("initialize: %v", err)
	}
	return response
}

func (h *harness) newSession(cwd string) sdk.NewSessionResponse {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := h.client.NewSession(ctx, sdk.NewSessionRequest{Cwd: cwd, McpServers: []sdk.McpServer{}})
	if err != nil {
		h.t.Fatalf("session/new: %v", err)
	}
	return response
}

func (h *harness) prompt(sessionID sdk.SessionId, text string) sdk.PromptResponse {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := h.client.Prompt(ctx, sdk.PromptRequest{SessionId: sessionID, Prompt: []sdk.ContentBlock{sdk.TextBlock(text)}})
	if err != nil {
		h.t.Fatalf("session/prompt: %v", err)
	}
	return response
}

func baseOptions(chat acpagent.ChatRunner, sessions acpagent.SessionStore) acpagent.Options {
	return acpagent.Options{
		Chat:           chat,
		Sessions:       sessions,
		Models:         &fakeModels{models: []domain.LLMConfig{{ID: "model-1", Name: "Model One"}}},
		Uploads:        &fakeUploads{},
		PreferredModel: "Model One",
		ApprovalMode:   constants.ApprovalModeStandard,
		Version:        "test",
	}
}

func TestServeInitializeAndNewSession(t *testing.T) {
	cwd := t.TempDir()
	h := newHarness(t, baseOptions(&fakeChat{}, &fakeSessions{}))

	init := h.initialize()
	if init.ProtocolVersion != sdk.ProtocolVersionNumber {
		t.Fatalf("protocol version = %d", init.ProtocolVersion)
	}
	if init.AgentInfo == nil || init.AgentInfo.Name != "slimebot" {
		t.Fatalf("agent info = %+v", init.AgentInfo)
	}
	if !init.AgentCapabilities.LoadSession {
		t.Fatal("expected loadSession capability")
	}
	if !init.AgentCapabilities.PromptCapabilities.Image || !init.AgentCapabilities.PromptCapabilities.EmbeddedContext {
		t.Fatalf("prompt capabilities = %+v", init.AgentCapabilities.PromptCapabilities)
	}

	response := h.newSession(cwd)
	if response.SessionId == "" {
		t.Fatal("expected a session id")
	}
	if response.Modes == nil || len(response.Modes.AvailableModes) != 2 {
		t.Fatalf("modes = %+v", response.Modes)
	}
	ids := make(map[sdk.SessionConfigId]sdk.SessionConfigValueId)
	for _, option := range response.ConfigOptions {
		if option.Select != nil {
			ids[option.Select.Id] = option.Select.CurrentValue
		}
	}
	if ids["model"] != "model-1" {
		t.Fatalf("model option = %q (options=%+v)", ids["model"], response.ConfigOptions)
	}
	if ids["approvalMode"] != constants.ApprovalModeStandard {
		t.Fatalf("approval option = %q", ids["approvalMode"])
	}
	if _, ok := ids["thinkingLevel"]; !ok {
		t.Fatalf("missing thinking option: %+v", ids)
	}
	h.output.assertJSONRPC(t)
}

func TestServePromptStreamsUpdatesAndRequestsPermission(t *testing.T) {
	cwd := t.TempDir()
	approved := make(chan bool, 1)
	chat := &fakeChat{handler: func(ctx context.Context, callbacks chatsvc.AgentCallbacks) error {
		if callbacks.OnThinkingStart != nil {
			if err := callbacks.OnThinkingStart(chatsvc.ThinkingEventMeta{}); err != nil {
				return err
			}
		}
		if callbacks.OnThinkingChunk != nil {
			if err := callbacks.OnThinkingChunk("thinking", chatsvc.ThinkingEventMeta{}); err != nil {
				return err
			}
		}
		if callbacks.OnThinkingDone != nil {
			if err := callbacks.OnThinkingDone(chatsvc.ThinkingEventMeta{}); err != nil {
				return err
			}
		}
		if err := callbacks.OnChunk("hello "); err != nil {
			return err
		}
		request := chatsvc.ApprovalRequest{ToolCallID: "tool-1", ToolName: "exec", Command: "echo hi", RequiresApproval: true, Params: map[string]any{"command": "echo hi"}}
		if callbacks.OnToolCallStart != nil {
			if err := callbacks.OnToolCallStart(request); err != nil {
				return err
			}
		}
		answer, err := callbacks.WaitApproval(ctx, request.ToolCallID)
		if err != nil {
			return err
		}
		approved <- answer != nil && answer.Approved
		if callbacks.OnToolCallResult != nil {
			if err := callbacks.OnToolCallResult(chatsvc.ToolCallResult{ToolCallID: "tool-1", ToolName: "exec", Command: "echo hi", Status: constants.ToolCallStatusCompleted, Output: "hi"}); err != nil {
				return err
			}
		}
		if callbacks.OnTodoUpdate != nil {
			if err := callbacks.OnTodoUpdate(chatsvc.TodoUpdate{Items: []chatsvc.TodoItem{{ID: "1", Content: "verify", Status: "completed"}}}); err != nil {
				return err
			}
		}
		return callbacks.OnChunk("world")
	}}
	options := baseOptions(chat, &fakeSessions{})
	h := newHarness(t, options)
	h.initialize()
	session := h.newSession(cwd)

	response := h.prompt(session.SessionId, "run the tests")
	if response.StopReason != sdk.StopReasonEndTurn {
		t.Fatalf("stop reason = %q", response.StopReason)
	}
	select {
	case value := <-approved:
		if !value {
			t.Fatal("expected the permission bridge to approve the tool call")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("permission bridge was not exercised")
	}

	call := chat.lastCall(t)
	if call.content != "run the tests" || call.modelID != "model-1" || call.approvalMode != constants.ApprovalModeStandard {
		t.Fatalf("chat call = %+v", call)
	}

	updates, permissions := h.peer.snapshot()
	var thought, message, toolStart, toolDone, plan bool
	var messageText strings.Builder
	for _, update := range updates {
		switch {
		case update.AgentThoughtChunk != nil:
			if update.AgentThoughtChunk.Content.Text != nil && update.AgentThoughtChunk.Content.Text.Text == "thinking" {
				thought = true
			}
		case update.AgentMessageChunk != nil:
			if update.AgentMessageChunk.Content.Text != nil {
				messageText.WriteString(update.AgentMessageChunk.Content.Text.Text)
			}
		case update.ToolCall != nil:
			if update.ToolCall.ToolCallId == "tool-1" && update.ToolCall.Status == sdk.ToolCallStatusPending {
				toolStart = true
			}
		case update.ToolCallUpdate != nil:
			if update.ToolCallUpdate.ToolCallId == "tool-1" {
				status, _ := update.ToolCallUpdate.Status.Value()
				if status == sdk.ToolCallStatusCompleted {
					toolDone = true
				}
				if content, ok := update.ToolCallUpdate.Content.Value(); ok && len(content) > 0 && content[0].Content != nil && content[0].Content.Content.Text != nil && content[0].Content.Content.Text.Text == "hi" {
					toolDone = true
				}
			}
		case update.Plan != nil:
			if len(update.Plan.Entries) == 1 && update.Plan.Entries[0].Content == "verify" {
				plan = true
			}
		}
	}
	message = messageText.String() == "hello world"
	if !thought || !message || !toolStart || !toolDone || !plan {
		t.Fatalf("updates missing: thought=%v message=%v toolStart=%v toolDone=%v plan=%v text=%q", thought, message, toolStart, toolDone, plan, messageText.String())
	}
	if len(permissions) != 1 {
		t.Fatalf("permission requests = %d", len(permissions))
	}
	if permissions[0].SessionId != session.SessionId || permissions[0].ToolCall.ToolCallId != "tool-1" {
		t.Fatalf("permission request = %+v", permissions[0])
	}
	if len(permissions[0].Options) != 2 || permissions[0].Options[0].OptionId != "allow" || permissions[0].Options[1].OptionId != "reject" {
		t.Fatalf("permission options = %+v", permissions[0].Options)
	}
	h.output.assertJSONRPC(t)
}

func TestServePromptCancelStopsTurn(t *testing.T) {
	cwd := t.TempDir()
	started := make(chan struct{})
	chat := &fakeChat{handler: func(ctx context.Context, _ chatsvc.AgentCallbacks) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	h := newHarness(t, baseOptions(chat, &fakeSessions{}))
	h.initialize()
	session := h.newSession(cwd)

	type result struct {
		response sdk.PromptResponse
		err      error
	}
	results := make(chan result, 1)
	go func() {
		response, err := h.client.Prompt(context.Background(), sdk.PromptRequest{SessionId: session.SessionId, Prompt: []sdk.ContentBlock{sdk.TextBlock("wait")}})
		results <- result{response: response, err: err}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not start")
	}
	if err := h.client.Cancel(context.Background(), sdk.CancelNotification{SessionId: session.SessionId}); err != nil {
		t.Fatalf("session/cancel: %v", err)
	}
	select {
	case reply := <-results:
		if reply.err != nil {
			t.Fatalf("prompt after cancel: %v", reply.err)
		}
		if reply.response.StopReason != sdk.StopReasonCancelled {
			t.Fatalf("stop reason = %q", reply.response.StopReason)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt did not stop after session/cancel")
	}
	h.output.assertJSONRPC(t)
}

func TestServeLoadSessionReplaysHistory(t *testing.T) {
	cwd := t.TempDir()
	assistantID := "assistant-1"
	sessions := &fakeSessions{snapshot: domain.SessionExportSnapshot{
		Session: domain.Session{ID: "saved-session", WorkingDirectory: cwd},
		Messages: []domain.Message{
			{ID: "user-1", Role: "user", Content: "earlier question"},
			{ID: assistantID, Role: "assistant", Content: "answer <!-- TOOL_CALL:tool-9 --> done"},
		},
		ToolCalls: []domain.ToolCallRecord{{
			ToolCallID: "tool-9", ToolName: "read_file", Command: "cat", Status: constants.ToolCallStatusCompleted,
			Output: "body", AssistantMessageID: &assistantID,
		}},
	}}
	h := newHarness(t, baseOptions(&fakeChat{}, sessions))
	h.initialize()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := h.client.LoadSession(ctx, sdk.LoadSessionRequest{Cwd: cwd, SessionId: "saved-session", McpServers: []sdk.McpServer{}})
	if err != nil {
		t.Fatalf("session/load: %v", err)
	}
	if response.Modes == nil || len(response.Modes.AvailableModes) != 2 {
		t.Fatalf("load modes = %+v", response.Modes)
	}
	if sessions.replayID != "saved-session" {
		t.Fatalf("replayed session = %q", sessions.replayID)
	}

	updates, _ := h.peer.snapshot()
	var user, first, second, toolStart, toolDone bool
	for _, update := range updates {
		switch {
		case update.UserMessageChunk != nil:
			if update.UserMessageChunk.Content.Text != nil && update.UserMessageChunk.Content.Text.Text == "earlier question" {
				user = true
			}
		case update.AgentMessageChunk != nil:
			if update.AgentMessageChunk.Content.Text != nil {
				switch update.AgentMessageChunk.Content.Text.Text {
				case "answer ":
					first = true
				case " done":
					second = true
				}
			}
		case update.ToolCall != nil:
			if update.ToolCall.ToolCallId == "tool-9" {
				toolStart = true
			}
		case update.ToolCallUpdate != nil:
			if update.ToolCallUpdate.ToolCallId == "tool-9" {
				status, _ := update.ToolCallUpdate.Status.Value()
				toolDone = status == sdk.ToolCallStatusCompleted
			}
		}
	}
	if !user || !first || !second || !toolStart || !toolDone {
		t.Fatalf("replay updates missing: user=%v first=%v second=%v toolStart=%v toolDone=%v", user, first, second, toolStart, toolDone)
	}
	h.output.assertJSONRPC(t)
}
