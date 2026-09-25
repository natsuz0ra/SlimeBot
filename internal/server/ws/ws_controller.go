package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	chatsvc "slimebot/internal/services/chat"
	plansvc "slimebot/internal/services/plan"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Controller is the WebSocket handler: upgrades connections, splits read/write, serializes chat and tool approval.
type Controller struct {
	chatService *chatsvc.ChatService
	planService *plansvc.PlanService
	upgrader    websocket.Upgrader
}

// chatIncoming is the client WebSocket message shape.
type chatIncoming struct {
	Type            string   `json:"type"`            // Message type: chat, ping, tool_approve, etc.
	SessionID       string   `json:"sessionId"`       // Session ID
	MessageID       string   `json:"messageId"`       // Existing user message ID for edit-and-resend
	Content         string   `json:"content"`         // User input text
	DisplayContent  string   `json:"displayContent"`  // Optional user-visible text when content is an internal prompt
	ModelID         string   `json:"modelId"`         // LLM config ID
	AttachmentIDs   []string `json:"attachmentIds"`   // Attachment IDs
	ToolCallID      string   `json:"toolCallId"`      // Tool call ID (for approval flow)
	Approved        *bool    `json:"approved"`        // Approval outcome
	Answers         string   `json:"answers"`         // JSON-encoded answers for ask_questions tool
	ThinkingLevel   string   `json:"thinkingLevel"`   // Thinking level: off, low, medium, high
	PlanMode        bool     `json:"planMode"`        // Plan mode: LLM generates plan instead of executing
	PlanID          string   `json:"planId"`          // Plan ID (for approve/reject)
	SubagentModelID string   `json:"subagentModelId"` // User-selected subagent model override (empty = inherit)
}

type wsOutChunk struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
}

var wsChunkBufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

type activeChatCanceler struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// Set stores the cancel func for the active chat.
func (a *activeChatCanceler) Set(cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancel = cancel
}

// Clear clears the active cancel func if it matches.
func (a *activeChatCanceler) Clear(cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = cancel
	a.cancel = nil
}

// Cancel cancels the active chat; returns false if none is active.
func (a *activeChatCanceler) Cancel() bool {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func NewController(chatService *chatsvc.ChatService, planService *plansvc.PlanService, allowedOrigins ...string) *Controller {
	return &Controller{
		chatService: chatService,
		planService: planService,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				parsed, err := url.Parse(origin)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
					return false
				}
				if parsed.Host == r.Host {
					return true
				}
				for _, allowed := range allowedOrigins {
					if origin == allowed {
						return true
					}
				}
				return false
			},
		},
	}
}

// approvalBroker holds channels for tool-call approval.
type approvalBroker struct {
	// Guards channels map during concurrent access.
	mu sync.Mutex
	// toolCallID -> channel for approval response.
	channels map[string]chan chatsvc.ApprovalResponse
	// toolCallID -> approval response that arrived before WaitApproval registered.
	pending map[string]chatsvc.ApprovalResponse
}

func newApprovalBroker() *approvalBroker {
	return &approvalBroker{
		channels: make(map[string]chan chatsvc.ApprovalResponse),
		pending:  make(map[string]chatsvc.ApprovalResponse),
	}
}

// Register registers an approval channel for a tool call; returns the receive channel.
func (b *approvalBroker) Register(toolCallID string) chan chatsvc.ApprovalResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan chatsvc.ApprovalResponse, 1)
	if resp, ok := b.pending[toolCallID]; ok {
		ch <- resp
		delete(b.pending, toolCallID)
		return ch
	}
	b.channels[toolCallID] = ch
	return ch
}

// Resolve delivers an approval result to the registered channel.
func (b *approvalBroker) Resolve(toolCallID string, resp chatsvc.ApprovalResponse) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.channels[toolCallID]; ok {
		select {
		case ch <- resp:
		default:
		}
		delete(b.channels, toolCallID)
		return
	}
	b.pending[toolCallID] = resp
}

// Remove drops the approval channel for a tool call (timeout or cancel).
func (b *approvalBroker) Remove(toolCallID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.channels, toolCallID)
	delete(b.pending, toolCallID)
}

// Chat handles a WebSocket: upgrades HTTP, starts read/write loops and chat loop.
func (w *Controller) Chat(wr http.ResponseWriter, req *http.Request) {
	conn, err := w.upgrader.Upgrade(wr, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sessionCtx, cancelSession := context.WithCancel(req.Context())
	defer cancelSession()

	writeCh := make(chan any, constants.WSWriteChannelBuf)
	chatCh := make(chan chatIncoming, constants.WSChatChannelBuf)
	broker := newApprovalBroker()
	activeCancel := &activeChatCanceler{}

	enqueue := func(payload any) bool {
		select {
		case <-sessionCtx.Done():
			return false
		case writeCh <- payload:
			return true
		}
	}

	w.startWriteLoop(sessionCtx, cancelSession, conn, writeCh)
	w.startReadLoop(sessionCtx, cancelSession, conn, enqueue, chatCh, broker, activeCancel)
	w.runChatLoop(sessionCtx, enqueue, chatCh, broker, activeCancel)
}

// startWriteLoop runs a dedicated writer goroutine for ordered WebSocket sends.
func (w *Controller) startWriteLoop(
	sessionCtx context.Context,
	cancelSession context.CancelFunc,
	conn *websocket.Conn,
	writeCh <-chan any,
) {
	go func() {
		for {
			select {
			case <-sessionCtx.Done():
				return
			case payload := <-writeCh:
				if err := writePayload(conn, payload); err != nil {
					cancelSession()
					return
				}
			}
		}
	}()
}

// startReadLoop parses client messages and routes to chat queue or approval broker.
func (w *Controller) startReadLoop(
	sessionCtx context.Context,
	cancelSession context.CancelFunc,
	conn *websocket.Conn,
	enqueue func(any) bool,
	chatCh chan<- chatIncoming,
	broker *approvalBroker,
	activeCancel *activeChatCanceler,
) {
	// Read goroutine parses and routes; chat runs serially in the main loop.
	go func() {
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				cancelSession()
				return
			}

			var incoming chatIncoming
			if err := json.Unmarshal(payload, &incoming); err != nil {
				if !enqueue(map[string]any{"type": "error", "error": "Invalid message format."}) {
					cancelSession()
					return
				}
				continue
			}

			switch incoming.Type {
			case "ping":
				if !enqueue(map[string]any{"type": "pong"}) {
					cancelSession()
					return
				}
			case "tool_approve":
				if incoming.ToolCallID != "" && incoming.Approved != nil {
					broker.Resolve(incoming.ToolCallID, chatsvc.ApprovalResponse{
						ToolCallID: incoming.ToolCallID,
						Approved:   *incoming.Approved,
						Answers:    incoming.Answers,
					})
				}
			case "stop":
				// User stopped this stream: cancel chatCtx only, keep WebSocket open.
				if activeCancel.Cancel() {
					_ = enqueue(map[string]any{"type": "stopping", "sessionId": incoming.SessionID})
				}
			case "plan_approve":
				if w.planService != nil && incoming.PlanID != "" {
					plan, planErr := w.planService.UpdatePlanStatus(incoming.PlanID, constants.PlanStatusApproved)
					if planErr != nil {
						_ = enqueue(map[string]any{"type": "error", "error": "Plan not found."})
						continue
					}
					_ = enqueue(map[string]any{"type": "plan_status", "planId": plan.ID, "status": plan.Status})
					execContent := "Execute the following approved plan:\n\n" + plan.Content
					execIncoming := chatIncoming{
						Type:           "chat",
						SessionID:      incoming.SessionID,
						Content:        execContent,
						DisplayContent: incoming.DisplayContent,
						ModelID:        incoming.ModelID,
						PlanMode:       false,
					}
					select {
					case <-sessionCtx.Done():
						return
					case chatCh <- execIncoming:
					}
				}
			case "plan_reject":
				if w.planService != nil && incoming.PlanID != "" {
					_, _ = w.planService.UpdatePlanStatus(incoming.PlanID, constants.PlanStatusRejected)
					_ = enqueue(map[string]any{"type": "plan_status", "planId": incoming.PlanID, "status": constants.PlanStatusRejected})
				}
			case "plan_modify":
				if w.planService != nil && incoming.PlanID != "" {
					_, _ = w.planService.UpdatePlanStatus(incoming.PlanID, constants.PlanStatusRejected)
					_ = enqueue(map[string]any{"type": "plan_status", "planId": incoming.PlanID, "status": constants.PlanStatusRejected})
				}
				modifyIncoming := chatIncoming{
					Type:          "chat",
					SessionID:     incoming.SessionID,
					Content:       incoming.Content,
					ModelID:       incoming.ModelID,
					PlanMode:      true,
					ThinkingLevel: incoming.ThinkingLevel,
				}
				select {
				case <-sessionCtx.Done():
					return
				case chatCh <- modifyIncoming:
				}
			case "chat_edit":
				if strings.TrimSpace(incoming.Content) == "" || strings.TrimSpace(incoming.MessageID) == "" {
					continue
				}
				select {
				case <-sessionCtx.Done():
					return
				case chatCh <- incoming:
				}
			case "chat", "":
				if strings.TrimSpace(incoming.Content) == "" && len(incoming.AttachmentIDs) == 0 {
					continue
				}
				select {
				case <-sessionCtx.Done():
					return
				case chatCh <- incoming:
				}
			default:
				if !enqueue(map[string]any{"type": "error", "error": "Unsupported message type."}) {
					cancelSession()
					return
				}
			}
		}
	}()
}

// runChatLoop consumes chat messages serially to avoid per-connection races.
func (w *Controller) runChatLoop(
	sessionCtx context.Context,
	enqueue func(any) bool,
	chatCh <-chan chatIncoming,
	broker *approvalBroker,
	activeCancel *activeChatCanceler,
) {
	for {
		select {
		case <-sessionCtx.Done():
			return
		case incoming := <-chatCh:
			if !w.handleChatIncoming(sessionCtx, enqueue, broker, activeCancel, incoming) {
				return
			}
		}
	}
}

// handleChatIncoming handles one chat request and streams output plus completion events.
func (w *Controller) handleChatIncoming(
	sessionCtx context.Context,
	enqueue func(any) bool,
	broker *approvalBroker,
	activeCancel *activeChatCanceler,
	incoming chatIncoming,
) bool {
	receivedAt := time.Now()
	sessionID := strings.TrimSpace(incoming.SessionID)
	if sessionID == "" {
		return enqueue(map[string]any{
			"type":      "error",
			"sessionId": "",
			"error":     errors.New("sessionId is required").Error(),
		})
	}

	session, err := w.chatService.EnsureSession(sessionCtx, sessionID)
	if err != nil {
		return enqueue(map[string]any{
			"type":      "error",
			"sessionId": sessionID,
			"error":     err.Error(),
		})
	}
	if !enqueue(map[string]any{"type": "session", "sessionId": session.ID}) {
		return false
	}
	if incoming.Type != "chat_edit" && !enqueue(buildChatStartPayload(session.ID, receivedAt)) {
		return false
	}

	startSentAt := time.Now()
	var firstChunkSentAt time.Time
	requestID := uuid.NewString()
	chatCtx, cancel := context.WithTimeout(sessionCtx, constants.WSChatTimeout)
	activeCancel.Set(cancel)
	defer activeCancel.Clear(cancel)
	callbacks := w.buildCallbacks(enqueue, broker, session.ID, &firstChunkSentAt)
	var streamResult *chatsvc.ChatStreamResult
	if incoming.Type == "chat_edit" {
		startEnqueued := false
		callbacks.OnMessageEdited = func(messageID, content string, createdAt time.Time) error {
			if !enqueue(buildMessageEditedPayload(session.ID, messageID, content, createdAt)) {
				return context.Canceled
			}
			startSentAt = time.Now()
			startEnqueued = true
			if !enqueue(buildChatStartPayload(session.ID, startSentAt)) {
				return context.Canceled
			}
			return nil
		}
		streamResult, err = w.chatService.HandleEditedChatStream(
			chatCtx,
			session.ID,
			requestID,
			incoming.MessageID,
			incoming.Content,
			incoming.ModelID,
			incoming.ThinkingLevel,
			incoming.PlanMode,
			incoming.SubagentModelID,
			"",
			callbacks,
		)
		if err == nil && !startEnqueued {
			startSentAt = time.Now()
			if !enqueue(buildChatStartPayload(session.ID, startSentAt)) {
				return false
			}
		}
	} else {
		streamResult, err = w.chatService.HandleChatStreamWithReceivedAt(
			chatCtx,
			session.ID,
			requestID,
			receivedAt,
			incoming.Content,
			incoming.DisplayContent,
			incoming.ModelID,
			incoming.AttachmentIDs,
			incoming.ThinkingLevel,
			incoming.PlanMode,
			incoming.SubagentModelID,
			"",
			callbacks,
		)
	}
	cancel()

	if err != nil {
		return enqueue(map[string]any{
			"type":      "error",
			"sessionId": session.ID,
			"error":     err.Error(),
		})
	}
	if streamResult != nil && streamResult.PushFailed {
		if !enqueue(map[string]any{
			"type":      "error",
			"sessionId": session.ID,
			"error":     "Streaming interrupted, but the message has been saved.",
		}) {
			return false
		}
	}
	doneSentAt := time.Now()
	donePayload := buildChatDonePayload(session.ID, streamResult, receivedAt, doneSentAt)
	if streamResult != nil {
		if streamResult.PlanID != "" {
			donePayload["narration"] = streamResult.Narration
		}
	}
	if !enqueue(donePayload) {
		return false
	}
	if strings.TrimSpace(incoming.ModelID) != "" {
		usage, compactedNow, usageErr := w.chatService.GetContextUsageDetailed(sessionCtx, session.ID, incoming.ModelID)
		if usageErr != nil {
			logging.Warn("ws_context_usage_refresh_failed", "session", session.ID, "error", usageErr)
		} else {
			for _, payload := range buildPostDoneContextUsagePayloads(session.ID, usage, compactedNow) {
				if !enqueue(payload) {
					return false
				}
			}
		}
	}

	startToFirstChunkMs := int64(-1)
	firstChunkToDoneMs := int64(-1)
	if !firstChunkSentAt.IsZero() {
		startToFirstChunkMs = firstChunkSentAt.Sub(startSentAt).Milliseconds()
		firstChunkToDoneMs = doneSentAt.Sub(firstChunkSentAt).Milliseconds()
	}
	logging.Info("ws_chat_timing",
		"session", session.ID,
		"receive_to_start_ms", startSentAt.Sub(receivedAt).Milliseconds(),
		"start_to_first_chunk_ms", startToFirstChunkMs,
		"first_chunk_to_done_ms", firstChunkToDoneMs,
		"total_ms", doneSentAt.Sub(receivedAt).Milliseconds(),
	)
	return true
}

func buildChatStartPayload(sessionID string, startedAt time.Time) map[string]any {
	return map[string]any{
		"type":      "start",
		"sessionId": sessionID,
		"startedAt": startedAt.Format(time.RFC3339Nano),
	}
}

func buildMessageEditedPayload(sessionID, messageID, content string, createdAt time.Time) map[string]any {
	return map[string]any{
		"type":      "message_edited",
		"sessionId": sessionID,
		"messageId": messageID,
		"content":   content,
		"createdAt": createdAt.Format(time.RFC3339Nano),
	}
}

func buildChatDonePayload(sessionID string, streamResult *chatsvc.ChatStreamResult, receivedAt time.Time, doneSentAt time.Time) map[string]any {
	durationMs := doneSentAt.Sub(receivedAt).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	payload := map[string]any{
		"type":       "done",
		"sessionId":  sessionID,
		"finishedAt": doneSentAt.Format(time.RFC3339Nano),
		"durationMs": durationMs,
	}
	if streamResult == nil {
		return payload
	}
	payload["answer"] = streamResult.Answer
	payload["isInterrupted"] = streamResult.IsInterrupted
	payload["isStopPlaceholder"] = streamResult.IsStopPlaceholder
	if streamResult.PlanID != "" {
		payload["planId"] = streamResult.PlanID
		payload["planBody"] = streamResult.PlanBody
	}
	return payload
}

func buildTodoUpdatePayload(sessionID string, update chatsvc.TodoUpdate, updatedAt time.Time) map[string]any {
	payload := map[string]any{
		"type":      "todo_update",
		"sessionId": sessionID,
		"items":     update.Items,
		"updatedAt": updatedAt.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(update.Note) != "" {
		payload["note"] = update.Note
	}
	return payload
}

func buildContextUsagePayload(sessionID string, usage chatsvc.ContextUsage) map[string]any {
	return map[string]any{
		"type":             "context_usage",
		"sessionId":        sessionID,
		"modelConfigId":    usage.ModelConfigID,
		"usedTokens":       usage.UsedTokens,
		"totalTokens":      usage.TotalTokens,
		"usedPercent":      usage.UsedPercent,
		"availablePercent": usage.AvailablePercent,
		"isCompacted":      usage.IsCompacted,
		"compactedAt":      usage.CompactedAt,
	}
}

func buildContextCompactedPayload(sessionID string, usage chatsvc.ContextUsage) map[string]any {
	return map[string]any{
		"type":      "context_compacted",
		"sessionId": sessionID,
		"usage":     usage,
	}
}

func buildPostDoneContextUsagePayloads(sessionID string, usage chatsvc.ContextUsage, compactedNow bool) []map[string]any {
	payloads := []map[string]any{buildContextUsagePayload(sessionID, usage)}
	if compactedNow {
		payloads = append(payloads, buildContextCompactedPayload(sessionID, usage))
	}
	return payloads
}

func truncateWSString(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return trimmed
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func addTeamIdentity(payload map[string]any, teamRunID, memberRunID string) {
	if teamRunID != "" {
		payload["teamRunId"] = teamRunID
	}
	if memberRunID != "" {
		payload["memberRunId"] = memberRunID
	}
}

func buildSubagentStartPayload(sessionID string, meta chatsvc.AgentEventMeta, title, task string) map[string]any {
	payload := map[string]any{
		"type":             "subagent_start",
		"sessionId":        sessionID,
		"parentToolCallId": meta.ParentToolCallID,
		"subagentRunId":    meta.SubagentRunID,
		"title":            truncateWSString(title, 80),
		"task":             truncateWSString(task, 512),
	}
	addTeamIdentity(payload, meta.TeamRunID, meta.MemberRunID)
	return payload
}

func buildTeamStartPayload(run domain.TeamRun) map[string]any {
	return map[string]any{
		"type":        "team_start",
		"sessionId":   run.SessionID,
		"requestId":   run.RequestID,
		"teamRunId":   run.ID,
		"status":      run.Status,
		"maxMembers":  run.MaxMembers,
		"maxParallel": run.MaxParallel,
		"startedAt":   run.StartedAt.Format(time.RFC3339Nano),
	}
}

func buildTeamDonePayload(run domain.TeamRun) map[string]any {
	payload := buildTeamStartPayload(run)
	payload["type"] = "team_done"
	payload["lastError"] = run.LastError
	if run.FinishedAt != nil {
		payload["finishedAt"] = run.FinishedAt.Format(time.RFC3339Nano)
	}
	return payload
}

func buildTeamMemberQueuedPayload(sessionID string, member domain.TeamMemberRun) map[string]any {
	return map[string]any{
		"type":          "team_member_queued",
		"sessionId":     sessionID,
		"teamRunId":     member.TeamRunID,
		"memberRunId":   member.ID,
		"toolCallId":    member.ToolCallID,
		"title":         truncateWSString(member.Title, 80),
		"task":          truncateWSString(member.Task, 512),
		"modelConfigId": member.ModelConfigID,
		"status":        member.Status,
		"createdAt":     member.CreatedAt.Format(time.RFC3339Nano),
		"updatedAt":     member.UpdatedAt.Format(time.RFC3339Nano),
	}
}

// buildCallbacks builds ChatService callbacks and maps them to WebSocket events.
func (w *Controller) buildCallbacks(
	enqueue func(any) bool,
	broker *approvalBroker,
	sessionID string,
	firstChunkSentAt *time.Time,
) chatsvc.AgentCallbacks {
	return chatsvc.AgentCallbacks{
		OnChunk: func(chunk string) error {
			if firstChunkSentAt != nil && firstChunkSentAt.IsZero() && chunk != "" {
				*firstChunkSentAt = time.Now()
			}
			if !enqueueWSChunk(enqueue, sessionID, chunk) {
				return context.Canceled
			}
			return nil
		},
		OnContextUsage: func(usage chatsvc.ContextUsage) error {
			if !enqueue(buildContextUsagePayload(sessionID, usage)) {
				return context.Canceled
			}
			return nil
		},
		OnContextCompacted: func(usage chatsvc.ContextUsage) error {
			if !enqueue(buildContextCompactedPayload(sessionID, usage)) {
				return context.Canceled
			}
			return nil
		},
		OnThinkingStart: func(meta chatsvc.ThinkingEventMeta) error {
			payload := map[string]any{"type": "thinking_start", "sessionId": sessionID, "startedAt": time.Now().Format(time.RFC3339Nano)}
			if meta.ParentToolCallID != "" {
				payload["parentToolCallId"] = meta.ParentToolCallID
			}
			if meta.SubagentRunID != "" {
				payload["subagentRunId"] = meta.SubagentRunID
			}
			addTeamIdentity(payload, meta.TeamRunID, meta.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnThinkingChunk: func(chunk string, meta chatsvc.ThinkingEventMeta) error {
			payload := map[string]any{"type": "thinking_chunk", "sessionId": sessionID, "content": chunk, "startedAt": time.Now().Format(time.RFC3339Nano)}
			if meta.ParentToolCallID != "" {
				payload["parentToolCallId"] = meta.ParentToolCallID
			}
			if meta.SubagentRunID != "" {
				payload["subagentRunId"] = meta.SubagentRunID
			}
			addTeamIdentity(payload, meta.TeamRunID, meta.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnThinkingDone: func(meta chatsvc.ThinkingEventMeta) error {
			payload := map[string]any{"type": "thinking_done", "sessionId": sessionID, "finishedAt": time.Now().Format(time.RFC3339Nano)}
			if meta.ParentToolCallID != "" {
				payload["parentToolCallId"] = meta.ParentToolCallID
			}
			if meta.SubagentRunID != "" {
				payload["subagentRunId"] = meta.SubagentRunID
			}
			addTeamIdentity(payload, meta.TeamRunID, meta.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnTodoUpdate: func(update chatsvc.TodoUpdate) error {
			if !enqueue(buildTodoUpdatePayload(sessionID, update, time.Now())) {
				return context.Canceled
			}
			return nil
		},
		OnToolCallStart: func(req chatsvc.ApprovalRequest) error {
			payload := map[string]any{
				"type":             "tool_call_start",
				"sessionId":        sessionID,
				"toolCallId":       req.ToolCallID,
				"toolName":         req.ToolName,
				"command":          req.Command,
				"params":           req.Params,
				"requiresApproval": req.RequiresApproval,
				"reviewStatus":     req.ReviewStatus,
				"reviewRisk":       req.ReviewRisk,
				"reviewReason":     req.ReviewReason,
				"preamble":         req.Preamble,
				"startedAt":        time.Now().Format(time.RFC3339Nano),
			}
			if req.ParentToolCallID != "" {
				payload["parentToolCallId"] = req.ParentToolCallID
			}
			if req.SubagentRunID != "" {
				payload["subagentRunId"] = req.SubagentRunID
			}
			addTeamIdentity(payload, req.TeamRunID, req.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnToolApprovalReview: func(event chatsvc.ApprovalReviewEvent) error {
			payload := map[string]any{
				"type":         "tool_call_review",
				"sessionId":    sessionID,
				"toolCallId":   event.ToolCallID,
				"toolName":     event.ToolName,
				"command":      event.Command,
				"reviewStatus": event.ReviewStatus,
				"reviewRisk":   event.ReviewRisk,
				"reviewReason": event.ReviewReason,
				"reviewedAt":   time.Now().Format(time.RFC3339Nano),
			}
			if event.ParentToolCallID != "" {
				payload["parentToolCallId"] = event.ParentToolCallID
			}
			if event.SubagentRunID != "" {
				payload["subagentRunId"] = event.SubagentRunID
			}
			addTeamIdentity(payload, event.TeamRunID, event.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnToolApprovalRequired: func(req chatsvc.ApprovalRequest) error {
			payload := map[string]any{
				"type":             "tool_call_approval_required",
				"sessionId":        sessionID,
				"toolCallId":       req.ToolCallID,
				"toolName":         req.ToolName,
				"command":          req.Command,
				"params":           req.Params,
				"requiresApproval": req.RequiresApproval,
				"reviewStatus":     req.ReviewStatus,
				"reviewRisk":       req.ReviewRisk,
				"reviewReason":     req.ReviewReason,
				"requiredAt":       time.Now().Format(time.RFC3339Nano),
			}
			if req.ParentToolCallID != "" {
				payload["parentToolCallId"] = req.ParentToolCallID
			}
			if req.SubagentRunID != "" {
				payload["subagentRunId"] = req.SubagentRunID
			}
			addTeamIdentity(payload, req.TeamRunID, req.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		WaitApproval: func(ctx context.Context, toolCallID string) (*chatsvc.ApprovalResponse, error) {
			ch := broker.Register(toolCallID)
			defer broker.Remove(toolCallID)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case resp := <-ch:
				return &resp, nil
			}
		},
		OnToolCallResult: func(result chatsvc.ToolCallResult) error {
			payload := map[string]any{
				"type":             "tool_call_result",
				"sessionId":        sessionID,
				"toolCallId":       result.ToolCallID,
				"toolName":         result.ToolName,
				"command":          result.Command,
				"requiresApproval": result.RequiresApproval,
				"status":           result.Status,
				"output":           result.Output,
				"error":            result.Error,
				"metadata":         result.Metadata,
				"finishedAt":       time.Now().Format(time.RFC3339Nano),
			}
			if result.ParentToolCallID != "" {
				payload["parentToolCallId"] = result.ParentToolCallID
			}
			if result.SubagentRunID != "" {
				payload["subagentRunId"] = result.SubagentRunID
			}
			addTeamIdentity(payload, result.TeamRunID, result.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnTeamStart: func(run domain.TeamRun) error {
			if !enqueue(buildTeamStartPayload(run)) {
				return context.Canceled
			}
			return nil
		},
		OnTeamMemberQueued: func(member domain.TeamMemberRun) error {
			if !enqueue(buildTeamMemberQueuedPayload(sessionID, member)) {
				return context.Canceled
			}
			return nil
		},
		OnTeamDone: func(run domain.TeamRun) error {
			if !enqueue(buildTeamDonePayload(run)) {
				return context.Canceled
			}
			return nil
		},
		OnSubagentStart: func(meta chatsvc.AgentEventMeta, title, task string) error {
			if !enqueue(buildSubagentStartPayload(sessionID, meta, title, task)) {
				return context.Canceled
			}
			return nil
		},
		OnSubagentChunk: func(meta chatsvc.AgentEventMeta, chunk string) error {
			payload := map[string]any{
				"type":             "subagent_chunk",
				"sessionId":        sessionID,
				"parentToolCallId": meta.ParentToolCallID,
				"subagentRunId":    meta.SubagentRunID,
				"content":          chunk,
			}
			addTeamIdentity(payload, meta.TeamRunID, meta.MemberRunID)
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnSubagentDone: func(meta chatsvc.AgentEventMeta, runErr error) error {
			payload := map[string]any{
				"type":             "subagent_done",
				"sessionId":        sessionID,
				"parentToolCallId": meta.ParentToolCallID,
				"subagentRunId":    meta.SubagentRunID,
			}
			addTeamIdentity(payload, meta.TeamRunID, meta.MemberRunID)
			if runErr != nil {
				payload["error"] = runErr.Error()
			}
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnPlanStart: func() error {
			if !enqueue(map[string]any{
				"type":      "plan_start",
				"sessionId": sessionID,
			}) {
				return context.Canceled
			}
			return nil
		},
		OnPlanChunk: func(chunk string) error {
			if !enqueue(map[string]any{
				"type":      "plan_chunk",
				"sessionId": sessionID,
				"content":   chunk,
			}) {
				return context.Canceled
			}
			return nil
		},
		OnPlanBody: func(planBody string) error {
			if !enqueue(map[string]any{
				"type":      "plan_body",
				"sessionId": sessionID,
				"content":   planBody,
			}) {
				return context.Canceled
			}
			return nil
		},
		OnTitleGenerated: func(titleSessionID, title string) {
			enqueue(map[string]any{
				"type":      "session_title",
				"sessionId": titleSessionID,
				"title":     title,
			})
		},
	}
}

func writePayload(conn *websocket.Conn, payload any) error {
	switch v := payload.(type) {
	case *wsOutChunk:
		buf := wsChunkBufPool.Get().(*bytes.Buffer)
		buf.Reset()
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			wsChunkBufPool.Put(buf)
			return err
		}
		b := buf.Bytes()
		if len(b) > 0 && b[len(b)-1] == '\n' {
			b = b[:len(b)-1]
		}
		err := conn.WriteMessage(websocket.TextMessage, b)
		wsChunkBufPool.Put(buf)
		return err
	default:
		return conn.WriteJSON(payload)
	}
}

func enqueueWSChunk(enqueue func(any) bool, sessionID, chunk string) bool {
	return enqueue(&wsOutChunk{Type: "chunk", SessionID: sessionID, Content: chunk})
}
