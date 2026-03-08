package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"corner/backend/internal/services"

	"github.com/gorilla/websocket"
)

type WSController struct {
	chatService *services.ChatService
	upgrader    websocket.Upgrader
}

type chatIncoming struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
	ModelID   string `json:"modelId"`
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

	enqueue := func(payload map[string]any) bool {
		select {
		case <-sessionCtx.Done():
			return false
		case writeCh <- payload:
			return true
		}
	}

	// single writer goroutine: guarantees serialized websocket writes
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

	// reader goroutine: keeps processing ping/chat while a chat stream is running
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

			if incoming.Type == "ping" {
				if !enqueue(map[string]any{"type": "pong"}) {
					cancelSession()
					return
				}
				continue
			}

			if incoming.Type != "" && incoming.Type != "chat" {
				if !enqueue(map[string]any{"type": "error", "error": "不支持的消息类型"}) {
					cancelSession()
					return
				}
				continue
			}

			if strings.TrimSpace(incoming.Content) == "" {
				continue
			}

			select {
			case <-sessionCtx.Done():
				return
			case chatCh <- incoming:
			}
		}
	}()

	// chat processing loop: serially handles chat tasks for this connection
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
			if !enqueue(map[string]any{"type": "start", "sessionId": session.ID}) {
				return
			}
			startSentAt := time.Now()
			var firstChunkSentAt time.Time

			chatCtx, cancel := context.WithTimeout(sessionCtx, 120*time.Second)
			streamResult, err := w.chatService.HandleChatStream(chatCtx, session.ID, incoming.Content, incoming.ModelID, func(chunk string) error {
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
			})
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
				if !enqueue(map[string]any{
					"type":      "session_title",
					"sessionId": session.ID,
					"title":     streamResult.Title,
				}) {
					return
				}
			}
			if streamResult != nil && streamResult.PushFailed {
				if !enqueue(map[string]any{
					"type":      "error",
					"sessionId": session.ID,
					"error":     "推送中断，消息已保存",
				}) {
					return
				}
			}
			if !enqueue(map[string]any{"type": "done", "sessionId": session.ID}) {
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

func writeJSON(conn *websocket.Conn, payload map[string]any) error {
	return conn.WriteJSON(payload)
}
