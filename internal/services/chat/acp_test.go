package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	acpclient "slimebot/internal/acp/client"
	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"

	sdk "github.com/coder/acp-go-sdk"
)

func TestACPRoute(t *testing.T) {
	service := &ChatService{acp: &acpclient.Manager{}}
	session := &domain.Session{ID: "session-1"}
	agentID := "agent-1"
	session.ACPAgentID = &agentID

	if got, err := service.acpRoute(session, ""); err != nil || got != agentID {
		t.Fatalf("bound route = %q, %v", got, err)
	}
	if got, err := service.acpRoute(session, "acp:explicit"); err != nil || got != "explicit" {
		t.Fatalf("explicit route = %q, %v", got, err)
	}
	if got, err := service.acpRoute(session, "model-1"); err != nil || got != "" {
		t.Fatalf("local route = %q, %v", got, err)
	}
	if _, err := service.acpRoute(session, "acp:"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("empty explicit route error = %v", err)
	}
	if _, err := (&ChatService{}).acpRoute(session, "acp:explicit"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("unconfigured manager error = %v", err)
	}
	if got, err := (&ChatService{}).acpRoute(session, ""); err != nil || got != "" {
		t.Fatalf("unconfigured bound route = %q, %v", got, err)
	}
}

func TestACPPromptBlocks(t *testing.T) {
	root := t.TempDir()
	imagePath := filepath.Join(root, "pixel.png")
	imageData := []byte("\x89PNG\r\n\x1a\nfake-image")
	if err := os.WriteFile(imagePath, imageData, 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	textPath := filepath.Join(root, "notes.md")
	if err := os.WriteFile(textPath, []byte("# Notes\nhello"), 0o600); err != nil {
		t.Fatalf("write text: %v", err)
	}
	attachments := []UploadedAttachment{
		{ID: "image-1", Name: "pixel.png", Ext: "PNG", MimeType: "image/png", Category: attachmentCategoryImage, SizeBytes: int64(len(imageData)), Path: imagePath},
		{ID: "file-1", Name: "notes.md", Ext: "MD", MimeType: "text/markdown", Category: attachmentCategoryDocument, SizeBytes: 14, Path: textPath},
	}

	blocks := acpPromptBlocks("look", attachments, sdk.PromptCapabilities{Image: true, EmbeddedContext: true})
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d", len(blocks))
	}
	if blocks[0].Text == nil || blocks[0].Text.Text != "look" {
		t.Fatalf("first block = %+v", blocks[0])
	}
	if blocks[1].Image == nil || blocks[1].Image.MimeType != "image/png" {
		t.Fatalf("image block = %+v", blocks[1])
	}
	decoded, err := base64.StdEncoding.DecodeString(blocks[1].Image.Data)
	if err != nil || string(decoded) != string(imageData) {
		t.Fatalf("image data round trip failed: %v", err)
	}
	if blocks[2].Resource == nil || blocks[2].Resource.Resource.TextResourceContents == nil || blocks[2].Resource.Resource.TextResourceContents.Text != "# Notes\nhello" {
		t.Fatalf("resource block = %+v", blocks[2])
	}

	// Without embedded context the text attachment degrades to a text block.
	blocks = acpPromptBlocks("", attachments[1:], sdk.PromptCapabilities{})
	if len(blocks) != 1 || blocks[0].Text == nil {
		t.Fatalf("fallback blocks = %+v", blocks)
	}
	if blocks[0].Text.Text != "notes.md (text/markdown, 14 bytes)\n# Notes\nhello" {
		t.Fatalf("fallback text = %q", blocks[0].Text.Text)
	}

	// Images beyond the capability limit also degrade to metadata text.
	blocks = acpPromptBlocks("", attachments[:1], sdk.PromptCapabilities{})
	if len(blocks) != 1 || blocks[0].Text == nil || blocks[0].Text.Text != fmt.Sprintf("pixel.png (image/png, %d bytes)", len(imageData)) {
		t.Fatalf("image fallback = %+v", blocks)
	}
	if blocks := acpPromptBlocks("", nil, sdk.PromptCapabilities{Image: true}); len(blocks) != 0 {
		t.Fatalf("empty blocks = %+v", blocks)
	}
}

type acpCallbackLog struct {
	chunks       []string
	thinking     []string
	thinkingDone int
	tools        []ApprovalRequest
	approvals    []ApprovalRequest
	results      []ToolCallResult
	todos        [][]TodoItem
}

func (log *acpCallbackLog) callbacks(answer *ApprovalResponse, waitErr error) AgentCallbacks {
	return AgentCallbacks{
		OnChunk: func(chunk string) error {
			log.chunks = append(log.chunks, chunk)
			return nil
		},
		OnThinkingStart: func(ThinkingEventMeta) error { return nil },
		OnThinkingChunk: func(chunk string, _ ThinkingEventMeta) error {
			log.thinking = append(log.thinking, chunk)
			return nil
		},
		OnThinkingDone: func(ThinkingEventMeta) error {
			log.thinkingDone++
			return nil
		},
		OnToolCallStart: func(req ApprovalRequest) error {
			log.tools = append(log.tools, req)
			return nil
		},
		OnToolApprovalRequired: func(req ApprovalRequest) error {
			log.approvals = append(log.approvals, req)
			return nil
		},
		WaitApproval: func(context.Context, string) (*ApprovalResponse, error) {
			return answer, waitErr
		},
		OnToolCallResult: func(result ToolCallResult) error {
			log.results = append(log.results, result)
			return nil
		},
		OnTodoUpdate: func(update TodoUpdate) error {
			log.todos = append(log.todos, update.Items)
			return nil
		},
	}
}

func newACPTestBridge(approvalMode string, log *acpCallbackLog) *acpStreamBridge {
	return &acpStreamBridge{callbacks: log.callbacks(&ApprovalResponse{Approved: true}, nil), approvalMode: approvalMode, tools: make(map[sdk.ToolCallId]*acpTool)}
}

func TestACPStreamBridgeUpdate(t *testing.T) {
	log := &acpCallbackLog{}
	bridge := newACPTestBridge(constants.ApprovalModeStandard, log)

	if err := bridge.update(sdk.UpdateAgentThoughtText("thinking-1")); err != nil {
		t.Fatalf("thought: %v", err)
	}
	if err := bridge.update(sdk.UpdateAgentThoughtText("thinking-2")); err != nil {
		t.Fatalf("thought: %v", err)
	}
	if err := bridge.update(sdk.UpdateAgentMessageText("hello")); err != nil {
		t.Fatalf("message: %v", err)
	}
	if err := bridge.update(sdk.StartToolCall("tool-1", "Read file",
		sdk.WithStartName("read_file"), sdk.WithStartKind(sdk.ToolKindRead),
		sdk.WithStartStatus(sdk.ToolCallStatusInProgress), sdk.WithStartRawInput(map[string]any{"path": "a.txt"}))); err != nil {
		t.Fatalf("tool start: %v", err)
	}
	if err := bridge.update(sdk.UpdateToolCall("tool-1", sdk.WithUpdateStatus(sdk.ToolCallStatusCompleted),
		sdk.WithUpdateContent([]sdk.ToolCallContent{sdk.ToolContent(sdk.TextBlock("file body"))}))); err != nil {
		t.Fatalf("tool update: %v", err)
	}
	if err := bridge.update(sdk.UpdatePlan(sdk.PlanEntry{Content: "step one", Status: sdk.PlanEntryStatusCompleted})); err != nil {
		t.Fatalf("plan: %v", err)
	}

	if len(log.thinking) != 2 || log.thinkingDone != 1 {
		t.Fatalf("thinking events = %v done=%d", log.thinking, log.thinkingDone)
	}
	if len(log.chunks) != 1 || log.chunks[0] != "hello" {
		t.Fatalf("chunks = %v", log.chunks)
	}
	if len(log.tools) != 1 || log.tools[0].ToolName != "read_file" || log.tools[0].Command != string(sdk.ToolKindRead) {
		t.Fatalf("tool starts = %+v", log.tools)
	}
	if params, _ := log.tools[0].Params["input"].(map[string]any); params["path"] != "a.txt" {
		t.Fatalf("tool params = %+v", log.tools[0].Params)
	}
	if len(log.results) != 1 || log.results[0].Status != constants.ToolCallStatusCompleted || log.results[0].Output != "file body" {
		t.Fatalf("tool results = %+v", log.results)
	}
	if len(log.todos) != 1 || len(log.todos[0]) != 1 || log.todos[0][0].Content != "step one" {
		t.Fatalf("todos = %+v", log.todos)
	}
}

func TestACPStreamBridgePermissionModes(t *testing.T) {
	request := sdk.RequestPermissionRequest{
		SessionId: "session-1",
		ToolCall: sdk.ToolCallUpdate{
			ToolCallId: "tool-2", Title: sdk.PatchSet("Run command"),
			Kind: sdk.PatchSet(sdk.ToolKindExecute), RawInput: sdk.PatchSet(json.RawMessage(`{"command":"ls"}`)),
		},
		Options: []sdk.PermissionOption{
			{OptionId: "allow", Name: "Allow", Kind: sdk.PermissionOptionKindAllowOnce},
			{OptionId: "reject", Name: "Reject", Kind: sdk.PermissionOptionKindRejectOnce},
		},
	}

	autoLog := &acpCallbackLog{}
	autoBridge := newACPTestBridge(constants.ApprovalModeAuto, autoLog)
	response, err := autoBridge.permission(context.Background(), request)
	if err != nil {
		t.Fatalf("auto permission: %v", err)
	}
	if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != "allow" {
		t.Fatalf("auto response = %+v", response.Outcome)
	}
	if len(autoLog.approvals) != 0 {
		t.Fatalf("auto mode should not ask the user: %+v", autoLog.approvals)
	}

	log := &acpCallbackLog{}
	bridge := newACPTestBridge(constants.ApprovalModeStandard, log)
	bridge.callbacks.WaitApproval = func(context.Context, string) (*ApprovalResponse, error) {
		return &ApprovalResponse{Approved: false}, nil
	}
	response, err = bridge.permission(context.Background(), request)
	if err != nil {
		t.Fatalf("manual permission: %v", err)
	}
	if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != "reject" {
		t.Fatalf("manual response = %+v", response.Outcome)
	}
	if len(log.approvals) != 1 || !log.approvals[0].RequiresApproval {
		t.Fatalf("approval events = %+v", log.approvals)
	}
	if len(log.results) != 1 || log.results[0].Status != constants.ToolCallStatusRejected {
		t.Fatalf("rejected tool results = %+v", log.results)
	}

	cancelled := &acpCallbackLog{}
	cancelledBridge := newACPTestBridge(constants.ApprovalModeStandard, cancelled)
	cancelledBridge.callbacks.WaitApproval = func(context.Context, string) (*ApprovalResponse, error) {
		return nil, context.Canceled
	}
	response, err = cancelledBridge.permission(context.Background(), request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled permission error = %v", err)
	}
	if response.Outcome.Cancelled == nil {
		t.Fatalf("cancelled permission response = %+v", response.Outcome)
	}
}
