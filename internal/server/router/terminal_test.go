package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"slimebot/internal/auth"
	"slimebot/internal/config"
	"slimebot/internal/server/controller"
	"slimebot/internal/server/ws"
	settingssvc "slimebot/internal/services/settings"
	"slimebot/internal/services/terminal"

	"github.com/gorilla/websocket"
)

type terminalTestSettings struct{ enabled bool }

func (s *terminalTestSettings) Terminal(context.Context) (settingssvc.TerminalSettings, error) {
	return settingssvc.TerminalSettings{Enabled: s.enabled}, nil
}

func (s *terminalTestSettings) UpdateTerminal(context.Context, *bool, *string) error {
	return nil
}

func TestTerminalRoutesAuthenticateAndCheckWebSocketOrigin(t *testing.T) {
	manager := terminal.NewManager()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.CloseAll(ctx)
	})
	policy := &terminalTestSettings{enabled: true}
	tokens, err := auth.NewTokenManager("test-terminal-secret", 5)
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Generate("test")
	if err != nil {
		t.Fatal(err)
	}
	httpController := &controller.HTTPController{}
	httpController.SetTerminalService(manager, policy)
	wsController := ws.NewController(nil, nil)
	wsController.SetTerminalService(manager, policy.Terminal)
	server := httptest.NewServer(New(config.Config{}, tokens, httpController, wsController, nil, RouterConfig{Headless: true}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/terminal/status")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status without token = %d", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/terminal/sessions", strings.NewReader(`{"cols":90,"rows":30}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info terminal.SessionInfo
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/terminal?terminalId=" + info.ID + "&token=" + token
	refused, wrongOrigin, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {"https://other.example"}})
	if refused != nil {
		_ = refused.Close()
	}
	if wrongOrigin != nil {
		_ = wrongOrigin.Body.Close()
	}
	if err == nil || wrongOrigin == nil || wrongOrigin.StatusCode != http.StatusForbidden {
		t.Fatalf("untrusted origin returned %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": {server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var frame struct {
		Type    string               `json:"type"`
		Session terminal.SessionInfo `json:"session"`
	}
	if err := conn.ReadJSON(&frame); err != nil || frame.Type != "ready" || frame.Session.ID != info.ID {
		t.Fatalf("ready frame %+v, %v", frame, err)
	}
}
