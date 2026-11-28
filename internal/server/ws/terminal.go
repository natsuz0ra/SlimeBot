package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"slimebot/internal/constants"
	settingssvc "slimebot/internal/services/settings"
	"slimebot/internal/services/terminal"

	"github.com/gorilla/websocket"
)

func (w *Controller) SetTerminalService(manager *terminal.Manager, settings func(context.Context) (settingssvc.TerminalSettings, error)) {
	w.terminal, w.terminalSettings = manager, settings
}

type terminalControl struct {
	Type     string                `json:"type"`
	Cols     int                   `json:"cols,omitempty"`
	Rows     int                   `json:"rows,omitempty"`
	ExitCode *int                  `json:"exitCode,omitempty"`
	Message  string                `json:"message,omitempty"`
	Session  *terminal.SessionInfo `json:"session,omitempty"`
}

func (w *Controller) Terminal(rw http.ResponseWriter, r *http.Request) {
	if w.terminal == nil {
		http.NotFound(rw, r)
		return
	}
	policy, err := w.terminalSettings(r.Context())
	if err != nil {
		http.Error(rw, "Terminal unavailable.", http.StatusServiceUnavailable)
		return
	}
	if !policy.Enabled {
		http.Error(rw, "Terminal is disabled.", http.StatusForbidden)
		return
	}
	owner, _ := r.Context().Value(constants.ContextAuthUsername).(string)
	session, err := w.terminal.Get(owner, r.URL.Query().Get("terminalId"))
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	conn, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(64 << 10)
	info, replay, output, unsubscribe := session.Subscribe()
	defer unsubscribe()
	send := func(kind int, data []byte) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return conn.WriteMessage(kind, data)
	}
	control := func(frame terminalControl) error {
		data, _ := json.Marshal(frame)
		return send(websocket.TextMessage, data)
	}
	if control(terminalControl{Type: "ready", Session: &info}) != nil {
		return
	}
	if len(replay) > 0 && send(websocket.BinaryMessage, replay) != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	replies := make(chan terminalControl, 8)
	go func() {
		defer cancel()
		for {
			_ = conn.SetReadDeadline(time.Now().Add(75 * time.Second))
			kind, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var reply terminalControl
			switch kind {
			case websocket.BinaryMessage:
				err = session.Write(data)
			case websocket.TextMessage:
				var request terminalControl
				err = json.Unmarshal(data, &request)
				if err == nil {
					switch request.Type {
					case "ping":
						reply.Type = "pong"
					case "resize":
						err = session.Resize(request.Cols, request.Rows)
					}
				}
			}
			if err != nil {
				reply = terminalControl{Type: "error", Message: err.Error()}
			}
			if reply.Type != "" {
				select {
				case replies <- reply:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case reply := <-replies:
			if control(reply) != nil {
				return
			}
		case data, ok := <-output:
			if !ok {
				if end := session.Info(); end.Status == "exited" {
					_ = control(terminalControl{Type: "exit", ExitCode: end.ExitCode})
				}
				return
			}
			if send(websocket.BinaryMessage, data) != nil {
				return
			}
		}
	}
}
