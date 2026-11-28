package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"

	acpbridge "slimebot/internal/acp"
	acpclient "slimebot/internal/acp/client"
	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
)

type acpTurnClient interface {
	Prompt(ctx context.Context, session *domain.Session, agentID string, turn acpclient.Turn) (sdk.StopReason, error)
	ClearBinding(ctx context.Context, sessionID string, agentID *string) error
	Reset(ctx context.Context, session *domain.Session) error
}

func (s *ChatService) acpRoute(session *domain.Session, modelID string) (string, error) {
	modelID = strings.TrimSpace(modelID)
	if id, explicit := strings.CutPrefix(modelID, "acp:"); explicit {
		id = strings.TrimSpace(id)
		if id == "" || s.acp == nil {
			return "", fmt.Errorf("%w: invalid ACP agent selection", apperrors.ErrInvalidInput)
		}
		return id, nil
	}
	if s.acp != nil && modelID == "" && session.ACPAgentID != nil {
		return *session.ACPAgentID, nil
	}
	return "", nil
}

func (s *ChatService) unbindACPForLocalTurn(ctx context.Context, session *domain.Session) error {
	if s.acp == nil || session.ACPAgentID == nil {
		return nil
	}
	return s.acp.ClearBinding(ctx, session.ID, nil)
}

func (s *ChatService) ResetACPSession(ctx context.Context, sessionID string) error {
	if s.acp == nil {
		return apperrors.ErrNotFound
	}
	release, err := s.acquireTurnWithContext(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.acp.Reset(ctx, session)
}

func acpInterrupted(stop sdk.StopReason, err error) bool {
	return stop == sdk.StopReasonCancelled || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func acpPromptBlocks(text string, attachments []UploadedAttachment, capabilities sdk.PromptCapabilities) []sdk.ContentBlock {
	blocks := make([]sdk.ContentBlock, 0, len(attachments)+1)
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, sdk.TextBlock(text))
	}
	for _, file := range attachments {
		if capabilities.Image && file.Category == attachmentCategoryImage && file.SizeBytes <= maxChatUploadBytes {
			raw, err := os.ReadFile(file.Path)
			if err == nil {
				blocks = append(blocks, sdk.ImageBlock(base64.StdEncoding.EncodeToString(raw), file.MimeType))
				continue
			}
		}
		metadata := fmt.Sprintf("%s (%s, %d bytes)", file.Name, file.MimeType, file.SizeBytes)
		excerpt, ok := readAttachmentExcerpt(file.Path, file.MimeType, file.Ext)
		if capabilities.EmbeddedContext && ok {
			blocks = append(blocks, sdk.ResourceBlock(sdk.EmbeddedResourceResource{
				TextResourceContents: &sdk.TextResourceContents{
					Uri: "slimebot-attachment:" + url.PathEscape(file.Name), MimeType: &file.MimeType, Text: excerpt,
				},
			}))
			continue
		}
		if ok {
			metadata += "\n" + excerpt
		}
		blocks = append(blocks, sdk.TextBlock(metadata))
	}
	return blocks
}

type acpTool struct {
	request ApprovalRequest
	output  string
	denied  bool
}

type acpStreamBridge struct {
	mu           sync.Mutex
	callbacks    AgentCallbacks
	approvalMode string
	tools        map[sdk.ToolCallId]*acpTool
	thinking     bool
	closed       bool
}

type externalACPTurnEngine struct {
	agentID string
}

func (externalACPTurnEngine) capabilities() turnEngineCapabilities { return turnEngineCapabilities{} }

func (externalACPTurnEngine) emptyAnswer() string { return "The ACP agent returned no content." }

func (externalACPTurnEngine) prepare(ctx context.Context, s *ChatService, state *chatTurnState, req ChatTurnRequest) error {
	var user *domain.Message
	var err error
	if req.EditMessageID != "" {
		user, err = s.store.UpdateUserMessageAndPruneAfter(ctx, req.SessionID, req.EditMessageID, req.Content)
		if err != nil {
			return err
		}
		if err := s.acp.ClearBinding(ctx, req.SessionID, state.session.ACPAgentID); err != nil {
			return err
		}
		state.session.ACPRemoteSessionID = ""
		state.userCreatedAt = user.CreatedAt
	} else {
		state.attachments, err = s.resolveTurnAttachments(req.SessionID, req.AttachmentIDs)
		if err != nil {
			return err
		}
		user, err = s.persistUserMessage(ctx, req.SessionID, req.Content, req.DisplayContent, req.ReceivedAt, state.attachments)
		if err != nil {
			return err
		}
		state.userCreatedAt = req.ReceivedAt
	}
	state.userMessageID = user.ID
	state.userContent = req.Content
	return nil
}

func (e externalACPTurnEngine) run(ctx context.Context, s *ChatService, turn *chatTurn, approvalMode string, callbacks AgentCallbacks) (*chatTurnResult, error) {
	state := turn.state
	bridge := &acpStreamBridge{callbacks: callbacks, approvalMode: approvalMode, tools: make(map[sdk.ToolCallId]*acpTool)}
	stop, err := s.acp.Prompt(ctx, state.session, e.agentID, acpclient.Turn{
		PlanMode: turn.request.PlanMode,
		Content: func(capabilities sdk.PromptCapabilities) []sdk.ContentBlock {
			return acpPromptBlocks(state.userContent, state.attachments, capabilities)
		},
		Update: bridge.update, Permission: bridge.permission,
	})
	result := &chatTurnResult{interrupted: acpInterrupted(stop, err) || ctx.Err() != nil}
	bridge.mu.Lock()
	bridge.closed = true
	if !result.interrupted {
		if finishErr := bridge.finishThinking(); err == nil {
			err = finishErr
		}
	}
	bridge.mu.Unlock()
	if !result.interrupted {
		if finishErr := turn.recorder.finishOpenTools(ctx, "External tool did not report completion."); finishErr != nil {
			return result, finishErr
		}
	}
	return result, err
}

func (b *acpStreamBridge) finishThinking() error {
	if !b.thinking {
		return nil
	}
	b.thinking = false
	if b.callbacks.OnThinkingDone != nil {
		return b.callbacks.OnThinkingDone(ThinkingEventMeta{})
	}
	return nil
}

func (b *acpStreamBridge) startTool(id sdk.ToolCallId, title, name string, kind sdk.ToolKind, raw json.RawMessage) (*acpTool, error) {
	if tool := b.tools[id]; tool != nil {
		return tool, nil
	}
	if name == "" {
		name = "acp"
	}
	if title == "" {
		title = name
	}
	params := map[string]any{"title": title}
	if len(raw) > 0 {
		var input any
		if json.Unmarshal(raw, &input) == nil {
			params["input"] = input
		}
	}
	tool := &acpTool{request: ApprovalRequest{
		ToolCallID: uuid.NewString(), ToolName: name, Command: string(kind), Params: params,
	}}
	b.tools[id] = tool
	if b.callbacks.OnToolCallStart != nil {
		return tool, b.callbacks.OnToolCallStart(tool.request)
	}
	return tool, nil
}

func (b *acpStreamBridge) finishTool(tool *acpTool, status sdk.ToolCallStatus) error {
	if b.callbacks.OnToolCallResult == nil {
		return nil
	}
	req := tool.request
	result := ToolCallResult{ToolCallID: req.ToolCallID, ToolName: req.ToolName, Command: req.Command,
		RequiresApproval: req.RequiresApproval, Output: tool.output}
	switch {
	case tool.denied:
		result.Status = constants.ToolCallStatusRejected
	case status == sdk.ToolCallStatusCompleted:
		result.Status = constants.ToolCallStatusCompleted
	case status == sdk.ToolCallStatusFailed:
		result.Status, result.Error = constants.ToolCallStatusError, "External tool failed."
	default:
		return nil
	}
	return b.callbacks.OnToolCallResult(result)
}

func toolOutput(raw json.RawMessage, content []sdk.ToolCallContent) string {
	var chunks []string
	if len(raw) > 0 && string(raw) != "null" {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			text = string(raw)
		}
		chunks = append(chunks, text)
	}
	for _, item := range content {
		if item.Content != nil {
			chunks = append(chunks, acpbridge.ContentText(item.Content.Content))
		} else if item.Diff != nil {
			chunks = append(chunks, item.Diff.Path+"\n"+item.Diff.NewText)
		}
	}
	output := strings.TrimSpace(strings.Join(chunks, "\n"))
	if len(output) > 256<<10 {
		output = strings.ToValidUTF8(output[:256<<10], "") + "\n[truncated]"
	}
	return output
}

func (b *acpStreamBridge) update(update sdk.SessionUpdate) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	if chunk := update.AgentThoughtChunk; chunk != nil {
		meta := ThinkingEventMeta{}
		if !b.thinking {
			b.thinking = true
			if b.callbacks.OnThinkingStart != nil {
				if err := b.callbacks.OnThinkingStart(meta); err != nil {
					return err
				}
			}
		}
		if b.callbacks.OnThinkingChunk != nil {
			return b.callbacks.OnThinkingChunk(acpbridge.ContentText(chunk.Content), meta)
		}
		return nil
	}
	if err := b.finishThinking(); err != nil {
		return err
	}
	switch {
	case update.AgentMessageChunk != nil:
		if b.callbacks.OnChunk != nil {
			return b.callbacks.OnChunk(acpbridge.ContentText(update.AgentMessageChunk.Content))
		}
	case update.ToolCall != nil:
		call := update.ToolCall
		name := ""
		if call.Name != nil {
			name = *call.Name
		}
		tool, err := b.startTool(call.ToolCallId, call.Title, name, call.Kind, call.RawInput)
		if err != nil {
			return err
		}
		tool.output = toolOutput(call.RawOutput, call.Content)
		return b.finishTool(tool, call.Status)
	case update.ToolCallUpdate != nil:
		call := update.ToolCallUpdate
		title, _ := call.Title.Value()
		name, _ := call.Name.Value()
		kind, _ := call.Kind.Value()
		input, _ := call.RawInput.Value()
		tool, err := b.startTool(call.ToolCallId, title, name, kind, input)
		if err != nil {
			return err
		}
		output, gotOutput := call.RawOutput.Value()
		content, gotContent := call.Content.Value()
		if gotOutput || gotContent {
			tool.output = toolOutput(output, content)
		}
		status, _ := call.Status.Value()
		return b.finishTool(tool, status)
	case update.Plan != nil && b.callbacks.OnTodoUpdate != nil:
		items := make([]TodoItem, 0, len(update.Plan.Entries))
		for index, entry := range update.Plan.Entries {
			items = append(items, TodoItem{ID: fmt.Sprint(index + 1), Content: entry.Content, Status: string(entry.Status)})
		}
		return b.callbacks.OnTodoUpdate(TodoUpdate{Items: items})
	}
	return nil
}

func (b *acpStreamBridge) permission(ctx context.Context, request sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	b.mu.Lock()
	switch {
	case b.closed:
		b.mu.Unlock()
		return acpbridge.CancelledPermission(), nil
	case b.approvalMode == constants.ApprovalModeAuto:
		b.mu.Unlock()
		return acpbridge.SelectPermission(request.Options, true), nil
	case b.callbacks.OnToolApprovalRequired == nil || b.callbacks.WaitApproval == nil:
		b.mu.Unlock()
		return acpbridge.SelectPermission(request.Options, false), nil
	}
	call := request.ToolCall
	title, _ := call.Title.Value()
	name, _ := call.Name.Value()
	kind, _ := call.Kind.Value()
	input, _ := call.RawInput.Value()
	tool, err := b.startTool(call.ToolCallId, title, name, kind, input)
	if err == nil {
		tool.request.RequiresApproval = true
		err = b.callbacks.OnToolApprovalRequired(tool.request)
	}
	id := tool.request.ToolCallID
	b.mu.Unlock()
	if err != nil {
		return acpbridge.CancelledPermission(), err
	}
	answer, err := b.callbacks.WaitApproval(ctx, id)
	if err != nil || ctx.Err() != nil || answer == nil {
		return acpbridge.CancelledPermission(), err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return acpbridge.CancelledPermission(), nil
	}
	if !answer.Approved {
		tool.denied = true
		err = b.finishTool(tool, sdk.ToolCallStatusFailed)
	}
	return acpbridge.SelectPermission(request.Options, answer.Approved), err
}
