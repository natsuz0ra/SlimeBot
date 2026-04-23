package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slimebot/internal/constants"
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
	Type          string   `json:"type"`          // Message type: chat, ping, tool_approve, etc.
	SessionID     string   `json:"sessionId"`     // Session ID
	Content       string   `json:"content"`       // User input text
	ModelID       string   `json:"modelId"`       // LLM config ID
	AttachmentIDs []string `json:"attachmentIds"` // Attachment IDs
	ToolCallID    string   `json:"toolCallId"`    // Tool call ID (for approval flow)
	Approved      *bool    `json:"approved"`      // Approval outcome
	ThinkingLevel string   `json:"thinkingLevel"` // Thinking level: off, low, medium, high
	PlanMode      bool     `json:"planMode"`      // Plan mode: LLM generates plan instead of executing
	PlanID        string   `json:"planId"`        // Plan ID (for approve/reject)
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

func NewController(chatService *chatsvc.ChatService, planService *plansvc.PlanService) *Controller {
	return &Controller{
		chatService: chatService,
		planService: planService,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				return true
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
}

func newApprovalBroker() *approvalBroker {
	return &approvalBroker{channels: make(map[string]chan chatsvc.ApprovalResponse)}
}

// Register registers an approval channel for a tool call; returns the receive channel.
func (b *approvalBroker) Register(toolCallID string) chan chatsvc.ApprovalResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan chatsvc.ApprovalResponse, 1)
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
	}
}

// Remove drops the approval channel for a tool call (timeout or cancel).
func (b *approvalBroker) Remove(toolCallID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.channels, toolCallID)
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
						Type:      "chat",
						SessionID: incoming.SessionID,
						Content:   execContent,
						ModelID:   incoming.ModelID,
						PlanMode:  false,
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
	if !enqueue(map[string]any{"type": "start", "sessionId": session.ID}) {
		return false
	}

	startSentAt := time.Now()
	var firstChunkSentAt time.Time
	requestID := uuid.NewString()
	chatCtx, cancel := context.WithTimeout(sessionCtx, constants.WSChatTimeout)
	activeCancel.Set(cancel)
	defer activeCancel.Clear(cancel)
	callbacks := w.buildCallbacks(enqueue, broker, session.ID, &firstChunkSentAt)
	streamResult, err := w.chatService.HandleChatStream(
		chatCtx,
		session.ID,
		requestID,
		incoming.Content,
		incoming.ModelID,
		incoming.AttachmentIDs,
		incoming.ThinkingLevel,
		incoming.PlanMode,
		callbacks,
	)
	cancel()

	if err != nil {
		return enqueue(map[string]any{
			"type":      "error",
			"sessionId": session.ID,
			"error":     err.Error(),
		})
	}
	if streamResult != nil && streamResult.TitleUpdated {
		if !enqueue(map[string]any{
			"type":      "session_title",
			"sessionId": session.ID,
			"title":     streamResult.Title,
		}) {
			return false
		}
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
	donePayload := map[string]any{"type": "done", "sessionId": session.ID}
	if streamResult != nil {
		donePayload["answer"] = chatsvc.StripContentMarkers(streamResult.Answer)
		// Client uses flags for copy and rendering (e.g. i18n for "output stopped").
		donePayload["isInterrupted"] = streamResult.IsInterrupted
		donePayload["isStopPlaceholder"] = streamResult.IsStopPlaceholder
		if streamResult.PlanID != "" {
			donePayload["planId"] = streamResult.PlanID
			donePayload["planBody"] = streamResult.PlanBody
			donePayload["narration"] = streamResult.Narration
		}
	}
	if !enqueue(donePayload) {
		return false
	}

	doneSentAt := time.Now()
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
		OnThinkingStart: func() error {
			if !enqueue(map[string]any{"type": "thinking_start", "sessionId": sessionID}) {
				return context.Canceled
			}
			return nil
		},
		OnThinkingChunk: func(chunk string) error {
			if !enqueue(map[string]any{"type": "thinking_chunk", "sessionId": sessionID, "content": chunk}) {
				return context.Canceled
			}
			return nil
		},
		OnThinkingDone: func() error {
			if !enqueue(map[string]any{"type": "thinking_done", "sessionId": sessionID}) {
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
				"preamble":         req.Preamble,
			}
			if req.ParentToolCallID != "" {
				payload["parentToolCallId"] = req.ParentToolCallID
			}
			if req.SubagentRunID != "" {
				payload["subagentRunId"] = req.SubagentRunID
			}
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
			}
			if result.ParentToolCallID != "" {
				payload["parentToolCallId"] = result.ParentToolCallID
			}
			if result.SubagentRunID != "" {
				payload["subagentRunId"] = result.SubagentRunID
			}
			if !enqueue(payload) {
				return context.Canceled
			}
			return nil
		},
		OnSubagentStart: func(parentToolCallID, runID, task string) error {
			t := task
			if len(t) > 512 {
				t = t[:512] + "…"
			}
			if !enqueue(map[string]any{
				"type":             "subagent_start",
				"sessionId":        sessionID,
				"parentToolCallId": parentToolCallID,
				"subagentRunId":    runID,
				"task":             t,
			}) {
				return context.Canceled
			}
			return nil
		},
		OnSubagentChunk: func(parentToolCallID, runID, chunk string) error {
			if !enqueue(map[string]any{
				"type":             "subagent_chunk",
				"sessionId":        sessionID,
				"parentToolCallId": parentToolCallID,
				"subagentRunId":    runID,
				"content":          chunk,
			}) {
				return context.Canceled
			}
			return nil
		},
		OnSubagentDone: func(parentToolCallID, runID string, runErr error) error {
			payload := map[string]any{
				"type":             "subagent_done",
				"sessionId":        sessionID,
				"parentToolCallId": parentToolCallID,
				"subagentRunId":    runID,
			}
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
