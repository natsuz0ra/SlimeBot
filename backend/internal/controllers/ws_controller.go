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

	// 单写协程：保证 websocket 写操作串行化
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

	// 读协程负责协议解包与分流；实际聊天处理在主循环串行执行。
	// 读协程：处理 ping/chat/tool_approve 消息
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

	// 聊天处理循环
	for {
		select {
		case <-sessionCtx.Done():
			return
		case incoming := <-chatCh:
			receivedAt := time.Now()
			sessionID := strings.TrimSpace(incoming.SessionID)
			if sessionID == "" {
				if !enqueue(map[string]any{
					"type":      "error",
					"sessionId": "",
					"error":     errors.New("sessionId is required").Error(),
				}) {
					return
				}
				continue
			}
			session, err := w.chatService.EnsureSession(sessionID)
			if err != nil {
				if !enqueue(map[string]any{
					"type":      "error",
					"sessionId": sessionID,
					"error":     err.Error(),
				}) {
					return
				}
				continue
			}

			if !enqueue(map[string]any{"type": "session", "sessionId": session.ID}) {
				return
			}
			// 先发 start，再逐块下发 chunk，前端据此进入流式渲染状态。
			if !enqueue(map[string]any{"type": "start", "sessionId": session.ID}) {
				return
			}
			startSentAt := time.Now()
			var firstChunkSentAt time.Time

			chatCtx, cancel := context.WithTimeout(sessionCtx, 300*time.Second)

			callbacks := services.AgentCallbacks{
				OnChunk: func(chunk string) error {
					// 首块到达时间用于统计首包延迟。
					if firstChunkSentAt.IsZero() && chunk != "" {
						firstChunkSentAt = time.Now()
					}
					if !enqueue(map[string]any{
						"type":      "chunk",
						"sessionId": session.ID,
						"content":   chunk,
					}) {
						return context.Canceled
					}
					return nil
				},
				OnToolCallStart: func(req services.ApprovalRequest) error {
					// 通知前端进入工具审批流程。
					if !enqueue(map[string]any{
						"type":       "tool_call_start",
						"sessionId":  session.ID,
						"toolCallId": req.ToolCallID,
						"toolName":   req.ToolName,
						"command":    req.Command,
						"params":     req.Params,
						"preamble":   req.Preamble,
					}) {
						return context.Canceled
					}
					return nil
				},
				WaitApproval: func(ctx context.Context, toolCallID string) (*services.ApprovalResponse, error) {
					// 将工具调用挂起，等待前端回传 tool_approve 结果。
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
					// 回传工具执行结果，便于前端展示中间产物。
					if !enqueue(map[string]any{
						"type":       "tool_call_result",
						"sessionId":  session.ID,
						"toolCallId": result.ToolCallID,
						"toolName":   result.ToolName,
						"command":    result.Command,
						"output":     result.Output,
						"error":      result.Error,
					}) {
						return context.Canceled
					}
					return nil
				},
			}

			streamResult, err := w.chatService.HandleChatStream(chatCtx, session.ID, incoming.Content, incoming.ModelID, callbacks)
			cancel()

			if err != nil {
				if !enqueue(map[string]any{
					"type":      "error",
					"sessionId": session.ID,
					"error":     err.Error(),
				}) {
					return
				}
				continue
			}

			if streamResult != nil && streamResult.TitleUpdated {
				// 标题有更新时，单独推送会话标题事件以刷新列表展示。
				if !enqueue(map[string]any{
					"type":      "session_title",
					"sessionId": session.ID,
					"title":     streamResult.Title,
				}) {
					return
				}
			}
			if streamResult != nil && streamResult.PushFailed {
				// 上游推送失败但消息已落库，前端收到提示后可引导用户重连。
				if !enqueue(map[string]any{
					"type":      "error",
					"sessionId": session.ID,
					"error":     "推送中断，消息已保存",
				}) {
					return
				}
			}
			donePayload := map[string]any{"type": "done", "sessionId": session.ID}
			if streamResult != nil {
				donePayload["answer"] = streamResult.Answer
			}
			if !enqueue(donePayload) {
				return
			}
			doneSentAt := time.Now()

			startToFirstChunkMs := int64(-1)
			firstChunkToDoneMs := int64(-1)
			if !firstChunkSentAt.IsZero() {
				startToFirstChunkMs = firstChunkSentAt.Sub(startSentAt).Milliseconds()
				firstChunkToDoneMs = doneSentAt.Sub(firstChunkSentAt).Milliseconds()
			}
			log.Printf(
				// 记录端到端关键阶段耗时，便于定位首包慢或尾包慢问题。
				"ws_chat_timing session=%s receive_to_start_ms=%d start_to_first_chunk_ms=%d first_chunk_to_done_ms=%d total_ms=%d",
				session.ID,
				startSentAt.Sub(receivedAt).Milliseconds(),
				startToFirstChunkMs,
				firstChunkToDoneMs,
				doneSentAt.Sub(receivedAt).Milliseconds(),
			)
		}
	}
}

// writeJSON 作为统一写出入口，便于后续集中处理写链路增强（如埋点/压缩）。
func writeJSON(conn *websocket.Conn, payload map[string]any) error {
	return conn.WriteJSON(payload)
}
