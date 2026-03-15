package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"slimebot/backend/internal/services"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type WSController struct {
	// 聊天业务入口，负责会话与流式输出处理。
	chatService *services.ChatService
	// WebSocket 升级与连接参数配置。
	upgrader websocket.Upgrader
}

type chatIncoming struct {
	// 客户端消息类型：chat/ping/tool_approve 等。
	Type string `json:"type"`
	// 会话标识，用于路由到指定聊天上下文。
	SessionID string `json:"sessionId"`
	// 用户输入内容（聊天消息正文）。
	Content string `json:"content"`
	// 期望使用的模型标识。
	ModelID string `json:"modelId"`
	// 工具调用审批对应的调用 ID。
	ToolCallID string `json:"toolCallId"`
	// 工具审批结果；nil 表示未携带审批字段。
	Approved *bool `json:"approved"`
}

func NewWSController(chatService *services.ChatService) *WSController {
	return &WSController{
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
	channels map[string]chan services.ApprovalResponse
}

func newApprovalBroker() *approvalBroker {
	return &approvalBroker{channels: make(map[string]chan services.ApprovalResponse)}
}

func (b *approvalBroker) Register(toolCallID string) chan services.ApprovalResponse {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan services.ApprovalResponse, 1)
	b.channels[toolCallID] = ch
	return ch
}

func (b *approvalBroker) Resolve(toolCallID string, resp services.ApprovalResponse) {
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

func (b *approvalBroker) Remove(toolCallID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.channels, toolCallID)
}

func (w *WSController) Chat(wr http.ResponseWriter, req *http.Request) {
	conn, err := w.upgrader.Upgrade(wr, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sessionCtx, cancelSession := context.WithCancel(req.Context())
	defer cancelSession()

	writeCh := make(chan map[string]any, 128)
	chatCh := make(chan chatIncoming, 16)
	broker := newApprovalBroker()

	// 统一入队写消息：会话结束后不再发送，避免 goroutine 悬挂。
	enqueue := func(payload map[string]any) bool {
		select {
		case <-sessionCtx.Done():
			return false
		case writeCh <- payload:
			return true
		}
	}

	w.startWriteLoop(sessionCtx, cancelSession, conn, writeCh)
	w.startReadLoop(sessionCtx, cancelSession, conn, enqueue, chatCh, broker)
	w.runChatLoop(sessionCtx, enqueue, chatCh, broker)
}

// startWriteLoop 启动单独写协程，确保 websocket 发送顺序一致。
func (w *WSController) startWriteLoop(
	sessionCtx context.Context,
	cancelSession context.CancelFunc,
	conn *websocket.Conn,
	writeCh <-chan map[string]any,
) {
	// 单写协程：保证 websocket 写操作串行化。
	go func() {
		for {
			select {
			case <-sessionCtx.Done():
				return
			case payload := <-writeCh:
				if err := writeJSON(conn, payload); err != nil {
					cancelSession()
					return
				}
			}
		}
	}()
}

// startReadLoop 解析客户端协议消息，并分流到聊天队列或审批通道。
func (w *WSController) startReadLoop(
	sessionCtx context.Context,
	cancelSession context.CancelFunc,
	conn *websocket.Conn,
	enqueue func(map[string]any) bool,
	chatCh chan<- chatIncoming,
	broker *approvalBroker,
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
				if !enqueue(map[string]any{"type": "error", "error": "消息格式错误"}) {
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
					broker.Resolve(incoming.ToolCallID, services.ApprovalResponse{
						ToolCallID: incoming.ToolCallID,
						Approved:   *incoming.Approved,
					})
				}
			case "chat", "":
				if strings.TrimSpace(incoming.Content) == "" {
					continue
				}
				select {
				case <-sessionCtx.Done():
					return
				case chatCh <- incoming:
				}
			default:
				if !enqueue(map[string]any{"type": "error", "error": "不支持的消息类型"}) {
					cancelSession()
					return
				}
			}
		}
	}()
}

// runChatLoop 串行消费 chat 消息，避免同连接内并发处理导致状态错乱。
func (w *WSController) runChatLoop(
	sessionCtx context.Context,
	enqueue func(map[string]any) bool,
	chatCh <-chan chatIncoming,
	broker *approvalBroker,
) {
	for {
		select {
		case <-sessionCtx.Done():
			return
		case incoming := <-chatCh:
			if !w.handleChatIncoming(sessionCtx, enqueue, broker, incoming) {
				return
			}
		}
	}
}

// handleChatIncoming 处理单条 chat 请求并完成流式输出与收尾事件下发。
func (w *WSController) handleChatIncoming(
	sessionCtx context.Context,
	enqueue func(map[string]any) bool,
	broker *approvalBroker,
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

	session, err := w.chatService.EnsureSession(sessionID)
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
	callbacks := w.buildCallbacks(enqueue, broker, session.ID, &firstChunkSentAt)
	streamResult, err := w.chatService.HandleChatStream(chatCtx, session.ID, requestID, incoming.Content, incoming.ModelID, callbacks)
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
			"error":     "推送中断，消息已保存",
		}) {
			return false
		}
	}
	donePayload := map[string]any{"type": "done", "sessionId": session.ID}
	if streamResult != nil {
		donePayload["answer"] = streamResult.Answer
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
	log.Printf(
		"ws_chat_timing session=%s receive_to_start_ms=%d start_to_first_chunk_ms=%d first_chunk_to_done_ms=%d total_ms=%d",
		session.ID,
		startSentAt.Sub(receivedAt).Milliseconds(),
		startToFirstChunkMs,
		firstChunkToDoneMs,
		doneSentAt.Sub(receivedAt).Milliseconds(),
	)
	return true
}

// buildCallbacks 构建 chatService 所需回调，桥接为 websocket 协议事件。
func (w *WSController) buildCallbacks(
	enqueue func(map[string]any) bool,
	broker *approvalBroker,
	sessionID string,
	firstChunkSentAt *time.Time,
) services.AgentCallbacks {
	return services.AgentCallbacks{
		OnChunk: func(chunk string) error {
			if firstChunkSentAt != nil && firstChunkSentAt.IsZero() && chunk != "" {
				*firstChunkSentAt = time.Now()
			}
			if !enqueue(map[string]any{
				"type":      "chunk",
				"sessionId": sessionID,
				"content":   chunk,
			}) {
				return context.Canceled
			}
			return nil
		},
		OnToolCallStart: func(req services.ApprovalRequest) error {
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
		WaitApproval: func(ctx context.Context, toolCallID string) (*services.ApprovalResponse, error) {
			ch := broker.Register(toolCallID)
			defer broker.Remove(toolCallID)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case resp := <-ch:
				return &resp, nil
			}
		},
		OnToolCallResult: func(result services.ToolCallResult) error {
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

// writeJSON 作为统一写出入口，便于后续集中处理写链路增强（如埋点/压缩）。
func writeJSON(conn *websocket.Conn, payload map[string]any) error {
	return conn.WriteJSON(payload)
}
