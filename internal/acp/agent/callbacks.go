package agent

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"

	acpbridge "slimebot/internal/acp"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"

	sdk "github.com/coder/acp-go-sdk"
)

func (b *bridge) send(ctx context.Context, id sdk.SessionId, update sdk.SessionUpdate) error {
	conn, err := b.connection(ctx)
	if err != nil {
		return err
	}
	return conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: id, Update: update})
}

func (b *bridge) callbacks(ctx context.Context, id sdk.SessionId) chatsvc.AgentCallbacks {
	var mu sync.Mutex
	tools := make(map[string]chatsvc.ApprovalRequest)
	send := func(update sdk.SessionUpdate) error { return b.send(ctx, id, update) }
	storeTool := func(req chatsvc.ApprovalRequest) bool {
		mu.Lock()
		defer mu.Unlock()
		_, existed := tools[req.ToolCallID]
		tools[req.ToolCallID] = req
		return existed
	}
	return chatsvc.AgentCallbacks{
		OnChunk:         func(chunk string) error { return send(sdk.UpdateAgentMessageText(chunk)) },
		OnThinkingChunk: func(chunk string, _ chatsvc.ThinkingEventMeta) error { return send(sdk.UpdateAgentThoughtText(chunk)) },
		OnToolCallStart: func(req chatsvc.ApprovalRequest) error {
			if storeTool(req) {
				return nil
			}
			status := sdk.ToolCallStatusInProgress
			if req.RequiresApproval {
				status = sdk.ToolCallStatusPending
			}
			return send(toolStarted(req.ToolCallID, req.ToolName, req.Command, req.Params, status))
		},
		OnToolApprovalRequired: func(req chatsvc.ApprovalRequest) error {
			if storeTool(req) {
				return send(sdk.UpdateToolCall(sdk.ToolCallId(req.ToolCallID), sdk.WithUpdateStatus(sdk.ToolCallStatusPending)))
			}
			return send(toolStarted(req.ToolCallID, req.ToolName, req.Command, req.Params, sdk.ToolCallStatusPending))
		},
		WaitApproval: func(waitCtx context.Context, idTool string) (*chatsvc.ApprovalResponse, error) {
			mu.Lock()
			req := tools[idTool]
			mu.Unlock()
			conn, err := b.connection(waitCtx)
			if err != nil {
				return nil, err
			}
			params, _ := json.Marshal(req.Params)
			resp, err := conn.RequestPermission(waitCtx, sdk.RequestPermissionRequest{
				SessionId: id,
				ToolCall: sdk.ToolCallUpdate{ToolCallId: sdk.ToolCallId(idTool), Title: sdk.PatchSet(toolTitle(req.ToolName, req.Command)),
					Kind: sdk.PatchSet(acpbridge.ToolKind(req.ToolName, req.Command)), RawInput: sdk.PatchSet(json.RawMessage(params)),
					Status: sdk.PatchSet(sdk.ToolCallStatusPending)},
				Options: []sdk.PermissionOption{
					{OptionId: "allow", Name: "Allow once", Kind: sdk.PermissionOptionKindAllowOnce},
					{OptionId: "reject", Name: "Reject", Kind: sdk.PermissionOptionKindRejectOnce},
				},
			})
			if err != nil {
				return nil, err
			}
			selected := resp.Outcome.Selected
			return &chatsvc.ApprovalResponse{ToolCallID: idTool, Approved: selected != nil && selected.OptionId == "allow"}, nil
		},
		OnToolCallResult: func(result chatsvc.ToolCallResult) error {
			return send(toolFinished(result.ToolCallID, result.Status, result.Output, result.Error))
		},
		OnTodoUpdate: func(todo chatsvc.TodoUpdate) error {
			items := make([]sdk.PlanEntry, 0, len(todo.Items))
			for _, item := range todo.Items {
				items = append(items, sdk.PlanEntry{Content: item.Content, Priority: sdk.PlanEntryPriorityMedium, Status: sdk.PlanEntryStatus(item.Status)})
			}
			return send(sdk.UpdatePlan(items...))
		},
		OnPlanChunk: func(chunk string) error { return send(sdk.UpdateAgentMessageText(chunk)) },
		OnPlanBody: func(body string) error {
			return send(sdk.UpdatePlan(sdk.PlanEntry{Content: body, Priority: sdk.PlanEntryPriorityHigh, Status: sdk.PlanEntryStatusCompleted}))
		},
	}
}

func toolTitle(name, command string) string {
	if command == "" || command == name {
		return name
	}
	return name + " · " + command
}

func toolStarted(id, name, command string, params any, status sdk.ToolCallStatus) sdk.SessionUpdate {
	return sdk.StartToolCall(sdk.ToolCallId(id), toolTitle(name, command),
		sdk.WithStartName(name), sdk.WithStartKind(acpbridge.ToolKind(name, command)), sdk.WithStartStatus(status), sdk.WithStartRawInput(params))
}

func toolFinished(id, status, output, message string) sdk.SessionUpdate {
	acpStatus := sdk.ToolCallStatusInProgress
	switch status {
	case constants.ToolCallStatusCompleted:
		acpStatus = sdk.ToolCallStatusCompleted
	case constants.ToolCallStatusError, constants.ToolCallStatusRejected:
		acpStatus = sdk.ToolCallStatusFailed
	case constants.ToolCallStatusPending, constants.ToolCallStatusReviewing:
		acpStatus = sdk.ToolCallStatusPending
	}
	opts := []sdk.ToolCallUpdateOpt{sdk.WithUpdateStatus(acpStatus)}
	if output != "" || message != "" {
		output = strings.TrimSpace(strings.Join([]string{output, message}, "\n"))
		opts = append(opts, sdk.WithUpdateContent([]sdk.ToolCallContent{sdk.ToolContent(sdk.TextBlock(output))}))
	}
	return sdk.UpdateToolCall(sdk.ToolCallId(id), opts...)
}

var historyMarker = regexp.MustCompile(`<!-- (TOOL_CALL|THINKING):([^>\s]+) -->`)

func (b *bridge) replay(ctx context.Context, id sdk.SessionId, snapshot domain.SessionExportSnapshot) error {
	tools := make(map[string][]domain.ToolCallRecord)
	thoughts := make(map[string][]domain.ThinkingRecord)
	for _, tool := range snapshot.ToolCalls {
		if tool.AssistantMessageID != nil {
			key := *tool.AssistantMessageID
			tools[key] = append(tools[key], tool)
		}
	}
	for _, thought := range snapshot.Thinking {
		if thought.AssistantMessageID != nil {
			key := *thought.AssistantMessageID
			thoughts[key] = append(thoughts[key], thought)
		}
	}
	for _, message := range snapshot.Messages {
		switch message.Role {
		case "user":
			body := domain.StripContentMarkers(message.Content)
			if body != "" {
				if err := b.send(ctx, id, sdk.UpdateUserMessageText(body)); err != nil {
					return err
				}
			}
		case "assistant":
			if err := b.replayAssistant(ctx, id, message.Content, tools[message.ID], thoughts[message.ID]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *bridge) replayAssistant(ctx context.Context, id sdk.SessionId, body string, tools []domain.ToolCallRecord, thoughts []domain.ThinkingRecord) error {
	sent := make(map[string]bool)
	sendText := func(text string) error {
		if domain.StripContentMarkers(text) == "" {
			return nil
		}
		return b.send(ctx, id, sdk.UpdateAgentMessageText(domain.StripContentMarkers(text)))
	}
	sendTool := func(tool domain.ToolCallRecord) error {
		sent["TOOL_CALL:"+tool.ToolCallID] = true
		if err := b.send(ctx, id, toolStarted(tool.ToolCallID, tool.ToolName, tool.Command, json.RawMessage(tool.ParamsJSON), sdk.ToolCallStatusPending)); err != nil {
			return err
		}
		return b.send(ctx, id, toolFinished(tool.ToolCallID, tool.Status, tool.Output, tool.Error))
	}
	sendThought := func(thought domain.ThinkingRecord) error {
		sent["THINKING:"+thought.ThinkingID] = true
		return b.send(ctx, id, sdk.UpdateAgentThoughtText(thought.Content))
	}
	toolIDs := make(map[string]domain.ToolCallRecord, len(tools))
	thoughtIDs := make(map[string]domain.ThinkingRecord, len(thoughts))
	for _, tool := range tools {
		toolIDs[tool.ToolCallID] = tool
	}
	for _, thought := range thoughts {
		thoughtIDs[thought.ThinkingID] = thought
	}
	offset := 0
	for _, match := range historyMarker.FindAllStringSubmatchIndex(body, -1) {
		if err := sendText(body[offset:match[0]]); err != nil {
			return err
		}
		offset = match[1]
		kind, key := body[match[2]:match[3]], body[match[4]:match[5]]
		if sent[kind+":"+key] {
			continue
		}
		if kind == "TOOL_CALL" {
			if tool, ok := toolIDs[key]; ok {
				if err := sendTool(tool); err != nil {
					return err
				}
			}
		} else if thought, ok := thoughtIDs[key]; ok {
			if err := sendThought(thought); err != nil {
				return err
			}
		}
	}
	if err := sendText(body[offset:]); err != nil {
		return err
	}
	for _, tool := range tools {
		if !sent["TOOL_CALL:"+tool.ToolCallID] {
			if err := sendTool(tool); err != nil {
				return err
			}
		}
	}
	for _, thought := range thoughts {
		if !sent["THINKING:"+thought.ThinkingID] {
			if err := sendThought(thought); err != nil {
				return err
			}
		}
	}
	return nil
}
