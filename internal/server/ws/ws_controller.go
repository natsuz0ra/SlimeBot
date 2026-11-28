package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	chatsvc "slimebot/internal/services/chat"
	llmsvc "slimebot/internal/services/llm"
	plansvc "slimebot/internal/services/plan"
	settingssvc "slimebot/internal/services/settings"
	"slimebot/internal/services/terminal"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Controller is the WebSocket handler: upgrades connections, splits read/write, serializes chat and tool approval.
type Controller struct {
	terminal         *terminal.Manager
	terminalSettings func(context.Context) (settingssvc.TerminalSettings, error)
	wakeBrokers      sync.Map
	chatService      *chatsvc.ChatService
	planService      *plansvc.PlanService
	upgrader         websocket.Upgrader
}

// chatIncoming is the client WebSocket message shape.
type chatIncoming struct {
	dequeued        func()   `json:"-"`
	TurnID          string   `json:"turnId"`
	SubmissionID    string   `json:"submissionId"`
	Type            string   `json:"type"`           // Message type: chat, ping, tool_approve, etc.
	SessionID       string   `json:"sessionId"`      // Session ID
	MessageID       string   `json:"messageId"`      // Existing user message ID for edit-and-resend
	Content         string   `json:"content"`        // User input text
	DisplayContent  string   `json:"displayContent"` // Optional user-visible text when content is an internal prompt
	ModelID         string   `json:"modelId"`        // LLM config ID
	ACPAgentID      string   `json:"acpAgentId"`
	AttachmentIDs   []string `json:"attachmentIds"`   // Attachment IDs
	ToolCallID      string   `json:"toolCallId"`      // Tool call ID (for approval flow)
	Approved        *bool    `json:"approved"`        // Approval outcome
	Answers         string   `json:"answers"`         // JSON-encoded answers for ask_questions tool
	ThinkingLevel   string   `json:"thinkingLevel"`   // Thinking level: off, low, medium, high
	PlanMode        bool     `json:"planMode"`        // Plan mode: LLM generates plan instead of executing
	PlanID          string   `json:"planId"`          // Plan ID (for approve/reject)
	SubagentModelID string   `json:"subagentModelId"` // User-selected subagent model override (empty = inherit)
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
	w := &Controller{
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
	if chatService != nil {
		chatService.SetBackgroundCallbacks(func(sessionID string, publish func(any) bool) chatsvc.AgentCallbacks {
			broker := newApprovalBroker()
			var first time.Time
			send := func(payload any) bool {
				var approval wsToolApproval
				switch event := payload.(type) {
				case wsToolCallStart:
					approval = event.wsToolApproval
				case wsToolApprovalRequired:
					approval = event.wsToolApproval
				case wsToolCallResult:
					broker.Remove(event.ToolCallID)
				}
				if approval.RequiresApproval {
					w.wakeBrokers.Store(sessionID, broker)
					broker.Register(approval.ToolCallID)
				}
				return publish(payload)
			}
			return w.buildCallbacks(send, broker, sessionID, &first, time.Now)
		})
	}

	return w

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
	if existing := b.channels[toolCallID]; existing != nil {
		return existing
	}
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

// ResolveRegistered routes only a known background approval, without retaining
// unrelated foreground responses in a process-wide pending map.
func (b *approvalBroker) ResolveRegistered(toolCallID string, resp chatsvc.ApprovalResponse) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := b.channels[toolCallID]
	if ch == nil {
		return false
	}
	select {
	case ch <- resp:
	default:
	}
	return true
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

	var watchMu sync.Mutex
	var watched string
	var stopWatch context.CancelFunc
	watch := func(sessionID string) {
		watchMu.Lock()
		defer watchMu.Unlock()
		if sessionID == watched {
			return
		}
		if stopWatch != nil {
			stopWatch()
		}
		watched = sessionID
		if sessionID == "" {
			return
		}
		ctx, stop := context.WithCancel(sessionCtx)
		stopWatch = stop
		events, unsubscribe := w.chatService.SubscribeSessionEvents(sessionID)
		go func() {
			defer unsubscribe()
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-events:
					if !ok {
						cancelSession()
						return
					}
					if !enqueue(event) {
						return
					}
				}
			}
		}()
	}
	currentSession := func() string { watchMu.Lock(); defer watchMu.Unlock(); return watched }
	w.startWriteLoop(sessionCtx, cancelSession, conn, writeCh)
	w.startReadLoop(sessionCtx, cancelSession, conn, enqueue, chatCh, broker, activeCancel, watch, currentSession)
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
	watch func(string),
	currentSession func() string,
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
				if !enqueue(wsError{Type: "error", Error: "Invalid message format."}) {
					cancelSession()
					return
				}
				continue
			}

			if incoming.Type == "chat" || incoming.Type == "chat_edit" || incoming.Type == "watch_session" {
				watch(strings.TrimSpace(incoming.SessionID))
			}
			switch incoming.Type {
			case "watch_session":
				continue
			case "background_cancel":
				w.chatService.CancelBackground(strings.TrimSpace(incoming.SessionID))
				continue

			case "ping":
				if !enqueue(wsPong{Type: "pong"}) {
					cancelSession()
					return
				}
			case "tool_approve":
				if incoming.ToolCallID != "" && incoming.Approved != nil {
					response := chatsvc.ApprovalResponse{ToolCallID: incoming.ToolCallID, Approved: *incoming.Approved, Answers: incoming.Answers}
					sessionID := incoming.SessionID
					if sessionID == "" {
						sessionID = currentSession()
					}
					if value, ok := w.wakeBrokers.Load(sessionID); ok && value.(*approvalBroker).ResolveRegistered(incoming.ToolCallID, response) {
						continue
					}

					broker.Resolve(incoming.ToolCallID, chatsvc.ApprovalResponse{
						ToolCallID: incoming.ToolCallID,
						Approved:   *incoming.Approved,
						Answers:    incoming.Answers,
					})
				}
			case "steer":
				accepted, err := w.chatService.Steer(sessionCtx, strings.TrimSpace(incoming.SessionID), incoming.TurnID, incoming.SubmissionID, incoming.Content)
				if err != nil {
					_ = enqueue(wsSteerRejected{wsSessionEvent: wsSessionEvent{"steer_rejected", incoming.SessionID}, SubmissionID: incoming.SubmissionID, Content: incoming.Content, Error: err.Error()})
					continue
				}
				_ = enqueue(wsSteerAccepted{wsSessionEvent: wsSessionEvent{"steer_accepted", incoming.SessionID}, Change: accepted})
			case "stop":
				w.chatService.CancelBackground(strings.TrimSpace(incoming.SessionID))
				// User stopped this stream: cancel chatCtx only, keep WebSocket open.
				if activeCancel.Cancel() {
					_ = enqueue(wsStopping{wsSessionEvent{"stopping", incoming.SessionID}})
				}
			case "plan_approve":
				if incoming.PlanID != "" {
					plan, planErr := w.planService.UpdatePlanStatus(incoming.PlanID, constants.PlanStatusApproved)
					if planErr != nil {
						_ = enqueue(wsError{Type: "error", Error: "Plan not found."})
						continue
					}
					_ = enqueue(wsPlanStatus{Type: "plan_status", PlanID: plan.ID, Status: plan.Status})
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
				if incoming.PlanID != "" {
					_, _ = w.planService.UpdatePlanStatus(incoming.PlanID, constants.PlanStatusRejected)
					_ = enqueue(wsPlanStatus{Type: "plan_status", PlanID: incoming.PlanID, Status: constants.PlanStatusRejected})
				}
			case "plan_modify":
				if incoming.PlanID != "" {
					_, _ = w.planService.UpdatePlanStatus(incoming.PlanID, constants.PlanStatusRejected)
					_ = enqueue(wsPlanStatus{Type: "plan_status", PlanID: incoming.PlanID, Status: constants.PlanStatusRejected})
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
				incoming.dequeued = w.chatService.QueueForeground(incoming.SessionID)
				select {
				case <-sessionCtx.Done():
					incoming.dequeued()
					return
				case chatCh <- incoming:
				}
			default:
				if !enqueue(wsError{Type: "error", Error: "Unsupported message type."}) {
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
	if incoming.dequeued != nil {
		defer incoming.dequeued()
	}
	receivedAt := time.Now()
	sessionID := strings.TrimSpace(incoming.SessionID)
	if sessionID == "" {
		return enqueue(wsError{Type: "error", SessionID: new(""), Error: "sessionId is required"})
	}

	session, err := w.chatService.EnsureSession(sessionCtx, sessionID)
	if err != nil {
		return enqueue(wsError{Type: "error", SessionID: new(sessionID), Error: err.Error()})
	}
	if !enqueue(wsSession{wsSessionEvent{"session", session.ID}}) {
		return false
	}

	startSentAt := time.Now()
	var firstChunkSentAt time.Time
	requestID := uuid.NewString()
	chatCtx, cancel := context.WithTimeout(context.WithoutCancel(sessionCtx), constants.WSChatTimeout)
	delivered := make(chan struct{})
	defer close(delivered)
	chatCtx = chatsvc.WithTurnDelivery(chatCtx, delivered)

	activeCancel.Set(cancel)
	defer activeCancel.Clear(cancel)
	defer cancel()
	taskEnqueue := func(payload any) bool {
		if sessionCtx.Err() != nil {
			return true
		}
		ok := enqueue(payload)
		return ok || sessionCtx.Err() != nil
	}
	callbacks := w.buildCallbacks(taskEnqueue, broker, session.ID, &firstChunkSentAt, time.Now)
	startPayload := func(at time.Time) wsStart {
		return buildChatStartPayload(session.ID, requestID, at)
	}
	if incoming.Type != "chat_edit" {
		callbacks.OnTurnStarted = func(string) error {
			if incoming.dequeued != nil {
				incoming.dequeued()
			}
			if !taskEnqueue(startPayload(receivedAt)) {
				return context.Canceled
			}
			return nil
		}
	}

	if agentID := strings.TrimSpace(incoming.ACPAgentID); agentID != "" {
		incoming.ModelID = "acp:" + agentID
	}
	request := chatsvc.ChatTurnRequest{
		SessionID: session.ID, RequestID: requestID, ReceivedAt: receivedAt,
		Content: incoming.Content, DisplayContent: incoming.DisplayContent, ModelID: incoming.ModelID,
		AttachmentIDs: incoming.AttachmentIDs, ThinkingLevel: incoming.ThinkingLevel, PlanMode: incoming.PlanMode,
		SubagentModelID: incoming.SubagentModelID,
	}
	startEnqueued := false
	if incoming.Type == "chat_edit" {
		request.EditMessageID = incoming.MessageID
		callbacks.OnMessageEdited = func(messageID, content string, createdAt time.Time) error {
			if !enqueue(buildMessageEditedPayload(session.ID, messageID, content, createdAt)) {
				return context.Canceled
			}
			startSentAt = time.Now()
			startEnqueued = true
			if !enqueue(startPayload(startSentAt)) {
				return context.Canceled
			}
			return nil
		}
	}
	streamResult, err := w.chatService.RunTurn(chatCtx, request, callbacks)
	cancel()

	if err != nil {
		return enqueue(wsError{Type: "error", SessionID: new(session.ID), Error: err.Error()})
	}
	if incoming.Type == "chat_edit" && !startEnqueued {
		startSentAt = time.Now()
		if !enqueue(startPayload(startSentAt)) {
			return false
		}
	}
	if streamResult.PushFailed || streamResult.ExecutionError != "" {
		if !enqueue(wsError{Type: "error", SessionID: new(session.ID), Error: "Streaming interrupted, but the message has been saved."}) {
			return false
		}
	}
	doneSentAt := time.Now()
	donePayload := buildChatDonePayload(session.ID, streamResult, receivedAt, doneSentAt)
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

// buildCallbacks maps chat service callbacks to the WebSocket wire contract.
func (w *Controller) buildCallbacks(
	enqueue func(any) bool,
	broker *approvalBroker,
	sessionID string,
	firstChunkSentAt *time.Time,
	now func() time.Time,
) chatsvc.AgentCallbacks {
	emit := func(payload any) error {
		if !enqueue(payload) {
			return context.Canceled
		}
		return nil
	}
	stamp := func() string { return now().Format(time.RFC3339Nano) }
	return chatsvc.AgentCallbacks{
		OnPrivacy: func(summary llmsvc.PrivacySummary) error {
			return emit(wsPrivacy{wsSessionEvent{"privacy", sessionID}, summary})
		},
		OnChunk: func(chunk string) error {
			if firstChunkSentAt.IsZero() && chunk != "" {
				*firstChunkSentAt = now()
			}
			return emit(&wsOutChunk{wsSessionEvent{"chunk", sessionID}, chunk})
		},
		OnContextUsage: func(usage chatsvc.ContextUsage) error {
			return emit(buildContextUsagePayload(sessionID, usage))
		},
		OnContextCompacted: func(usage chatsvc.ContextUsage) error {
			return emit(buildContextCompactedPayload(sessionID, usage))
		},
		OnThinkingStart: func(meta chatsvc.ThinkingEventMeta) error {
			return emit(wsThinkingStart{wsSessionEvent{"thinking_start", sessionID}, chatsvc.AgentEventMeta(meta), stamp()})
		},
		OnThinkingChunk: func(chunk string, meta chatsvc.ThinkingEventMeta) error {
			return emit(wsThinkingChunk{wsSessionEvent{"thinking_chunk", sessionID}, chatsvc.AgentEventMeta(meta), chunk, stamp()})
		},
		OnThinkingDone: func(meta chatsvc.ThinkingEventMeta) error {
			return emit(wsThinkingDone{wsSessionEvent{"thinking_done", sessionID}, chatsvc.AgentEventMeta(meta), stamp()})
		},
		OnTodoUpdate: func(update chatsvc.TodoUpdate) error {
			return emit(buildTodoUpdatePayload(sessionID, update, now()))
		},
		OnToolCallStart: func(req chatsvc.ApprovalRequest) error {
			return emit(wsToolCallStart{
				wsSessionEvent: wsSessionEvent{"tool_call_start", sessionID},
				wsToolApproval: toolApproval(req),
				Preamble:       req.Preamble, StartedAt: stamp(),
			})
		},
		OnToolApprovalReview: func(event chatsvc.ApprovalReviewEvent) error {
			return emit(wsToolCallReview{
				wsSessionEvent: wsSessionEvent{"tool_call_review", sessionID},
				wsToolReview: wsToolReview{
					wsToolIdentity: wsToolIdentity{
						ToolCallID: event.ToolCallID, ToolName: event.ToolName, Command: event.Command,
						AgentEventMeta: chatsvc.AgentEventMeta{
							ParentToolCallID: event.ParentToolCallID, SubagentRunID: event.SubagentRunID,
							TeamRunID: event.TeamRunID, MemberRunID: event.MemberRunID,
						},
					},
					ReviewStatus: event.ReviewStatus, ReviewRisk: event.ReviewRisk, ReviewReason: event.ReviewReason,
				},
				ReviewedAt: stamp(),
			})
		},
		OnToolApprovalRequired: func(req chatsvc.ApprovalRequest) error {
			return emit(wsToolApprovalRequired{
				wsSessionEvent: wsSessionEvent{"tool_call_approval_required", sessionID},
				wsToolApproval: toolApproval(req), RequiredAt: stamp(),
			})
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
			return emit(wsToolCallResult{
				wsSessionEvent: wsSessionEvent{"tool_call_result", sessionID},
				wsToolIdentity: wsToolIdentity{
					ToolCallID: result.ToolCallID, ToolName: result.ToolName, Command: result.Command,
					AgentEventMeta: chatsvc.AgentEventMeta{
						ParentToolCallID: result.ParentToolCallID, SubagentRunID: result.SubagentRunID,
						TeamRunID: result.TeamRunID, MemberRunID: result.MemberRunID,
					},
				},
				RequiresApproval: result.RequiresApproval, Status: result.Status, Output: result.Output,
				Error: result.Error, Metadata: result.Metadata, FinishedAt: stamp(),
			})
		},
		OnTeamStart: func(run domain.TeamRun) error { return emit(buildTeamStartPayload(run)) },
		OnTeamMemberQueued: func(member domain.TeamMemberRun) error {
			return emit(buildTeamMemberQueuedPayload(sessionID, member))
		},
		OnTeamDone: func(run domain.TeamRun) error { return emit(buildTeamDonePayload(run)) },
		OnSubagentStart: func(meta chatsvc.AgentEventMeta, title, task string) error {
			return emit(buildSubagentStartPayload(sessionID, meta, title, task))
		},
		OnSubagentChunk: func(meta chatsvc.AgentEventMeta, chunk string) error {
			return emit(wsSubagentChunk{wsSessionEvent{"subagent_chunk", sessionID}, subagentIdentity(meta), chunk})
		},
		OnSubagentDone: func(meta chatsvc.AgentEventMeta, runErr error) error {
			event := wsSubagentDone{wsSessionEvent: wsSessionEvent{"subagent_done", sessionID}, wsSubagentIdentity: subagentIdentity(meta)}
			if runErr != nil {
				event.Error = new(runErr.Error())
			}
			return emit(event)
		},
		OnPlanStart: func() error {
			return emit(wsPlanStart{wsSessionEvent{"plan_start", sessionID}})
		},
		OnPlanChunk: func(chunk string) error {
			return emit(wsPlanChunk{wsSessionEvent{"plan_chunk", sessionID}, chunk})
		},
		OnPlanBody: func(planBody string) error {
			return emit(wsPlanBody{wsSessionEvent{"plan_body", sessionID}, planBody})
		},
		OnTitleGenerated: func(titleSessionID, title string) {
			enqueue(wsSessionTitle{wsSessionEvent{"session_title", titleSessionID}, title})
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
		_ = enc.Encode(v)
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
