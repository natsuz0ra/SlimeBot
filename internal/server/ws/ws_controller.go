package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	chatsvc "slimebot/internal/services/chat"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Controller WebSocket 控制器：处理 WebSocket 连接升级、读写分离、串行处理聊天与工具审批
type Controller struct {
	chatService *chatsvc.ChatService
	upgrader    websocket.Upgrader
}

// chatIncoming 客户端发送的 WebSocket 消息结构
type chatIncoming struct {
	Type          string   `json:"type"`          // 消息类型：chat/ping/tool_approve 等
	SessionID     string   `json:"sessionId"`     // 会话ID
	Content       string   `json:"content"`       // 用户输入内容
	ModelID       string   `json:"modelId"`       // 模型配置ID
	AttachmentIDs []string `json:"attachmentIds"` // 附件ID列表
	ToolCallID    string   `json:"toolCallId"`    // 工具调用ID（用于审批）
	Approved      *bool    `json:"approved"`      // 审批结果
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

// Set 设置当前活跃的聊天取消函数
func (a *activeChatCanceler) Set(cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancel = cancel
}

// Clear 清除当前活跃的取消函数
func (a *activeChatCanceler) Clear(cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = cancel
	a.cancel = nil
}

// Cancel 取消当前活跃的聊天任务，若无活跃任务返回 false
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

func NewController(chatService *chatsvc.ChatService) *Controller {
	return &Controller{
		chatService: chatService,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

// approvalBroker 管理工具调用审批的通道
type approvalBroker struct {
	// 保护 channels，避免并发读写 map。
	mu sync.Mutex
	// toolCallID -> 审批回传通道。
	channels map[string]chan chatsvc.ApprovalResponse
}

func newApprovalBroker() *approvalBroker {
	return &approvalBroker{channels: make(map[string]chan chatsvc.ApprovalResponse)}
}

// Register 注册工具调用审批通道，返回用于接收审批结果的通道
func (b *approvalBroker) Register(toolCallID string) chan chatsvc.ApprovalResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan chatsvc.ApprovalResponse, 1)
	b.channels[toolCallID] = ch
	return ch
}

// Resolve 处理工具调用审批结果，将结果发送到对应通道
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

// Remove 移除指定工具调用的审批通道（超时或取消时调用）
func (b *approvalBroker) Remove(toolCallID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.channels, toolCallID)
}

// Chat 处理 WebSocket 连接：升级 HTTP 连接，启动读写循环和聊天处理循环
func (w *Controller) Chat(wr http.ResponseWriter, req *http.Request) {
	conn, err := w.upgrader.Upgrade(wr, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sessionCtx, cancelSession := context.WithCancel(req.Context())
	defer cancelSession()

	writeCh := make(chan any, 128)
	chatCh := make(chan chatIncoming, 16)
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

// startWriteLoop 启动单独写协程，确保 websocket 发送顺序一致。
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

// startReadLoop 解析客户端协议消息，并分流到聊天队列或审批通道。
func (w *Controller) startReadLoop(
	sessionCtx context.Context,
	cancelSession context.CancelFunc,
	conn *websocket.Conn,
	enqueue func(any) bool,
	chatCh chan<- chatIncoming,
	broker *approvalBroker,
	activeCancel *activeChatCanceler,
) {
	// 读协程负责协议解包与分流；实际聊天处理在主循环串行执行。
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
				// 用户主动中断本次流式输出：仅取消当前 chatCtx，不关闭 websocket 会话。
				if activeCancel.Cancel() {
					_ = enqueue(map[string]any{"type": "stopping", "sessionId": incoming.SessionID})
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

// runChatLoop 串行消费 chat 消息，避免同连接内并发处理导致状态错乱。
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

// handleChatIncoming 处理单条 chat 请求并完成流式输出与收尾事件下发。
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
	chatCtx, cancel := context.WithTimeout(sessionCtx, 600*time.Second)
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
		donePayload["answer"] = streamResult.Answer
		// 由前端根据标记决定文案与渲染（例如“已停止输出”多语言展示）。
		donePayload["isInterrupted"] = streamResult.IsInterrupted
		donePayload["isStopPlaceholder"] = streamResult.IsStopPlaceholder
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

// buildCallbacks 构建 chatService 所需回调，桥接为 websocket 协议事件。
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
		OnToolCallStart: func(req chatsvc.ApprovalRequest) error {
			if !enqueue(map[string]any{
				"type":             "tool_call_start",
				"sessionId":        sessionID,
				"toolCallId":       req.ToolCallID,
				"toolName":         req.ToolName,
				"command":          req.Command,
				"params":           req.Params,
				"requiresApproval": req.RequiresApproval,
				"preamble":         req.Preamble,
			}) {
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
			if !enqueue(map[string]any{
				"type":             "tool_call_result",
				"sessionId":        sessionID,
				"toolCallId":       result.ToolCallID,
				"toolName":         result.ToolName,
				"command":          result.Command,
				"requiresApproval": result.RequiresApproval,
				"status":           result.Status,
				"output":           result.Output,
				"error":            result.Error,
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
