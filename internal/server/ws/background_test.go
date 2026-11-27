package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	chat "slimebot/internal/services/chat"
	llm "slimebot/internal/services/llm"
	"slimebot/internal/tools"
)

type wakeSocketProvider struct {
	mu     sync.Mutex
	calls  int
	finish <-chan struct{}
}

func (p *wakeSocketProvider) StreamChatWithTools(ctx context.Context, _ llm.ModelRuntimeConfig, messages []llm.ChatMessage, defs []llm.ToolDef, cb llm.StreamCallbacks) (*llm.StreamResult, error) {
	if len(defs) == 0 {
		return &llm.StreamResult{Type: llm.StreamResultText}, nil
	}
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		observer, ok := tools.BackgroundObserverFromContext(ctx)
		if !ok {
			return nil, context.Canceled
		}
		event := tools.ProcessCompletion{ID: "proc-ws", Command: "synthetic", Status: "completed", Output: "synthetic background output", AutoWake: true}
		observer.Started(event)
		go func() { <-p.finish; observer.Completed(event) }()
		if cb.OnChunk != nil {
			_ = cb.OnChunk("已放到后台")
		}
		return &llm.StreamResult{Type: llm.StreamResultText}, nil
	}
	if call == 2 {
		if messages[len(messages)-1].Role != "tool" || !strings.Contains(messages[len(messages)-1].Content, "synthetic background output") {
			return nil, context.Canceled
		}
		tool := llm.ToolCallInfo{ID: "wake-needs-approval", Name: "exec__run", Arguments: `{"command":"Write-Output must-not-run","description":"synthetic approval check"}`}
		return &llm.StreamResult{Type: llm.StreamResultToolCalls, ToolCalls: []llm.ToolCallInfo{tool}, AssistantMessage: llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCallInfo{tool}}}, nil
	}
	if cb.OnChunk != nil {
		if err := cb.OnChunk("后续操作已拒绝，保留后台结果。"); err != nil {
			return nil, err
		}
	}
	return &llm.StreamResult{Type: llm.StreamResultText}, nil
}

func TestBackgroundWakeSurvivesDisconnectAndApprovalReconnect(t *testing.T) {
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "background_ws"))
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "synthetic background")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{Name: "synthetic", Provider: llm.ProviderOpenAI, BaseURL: "http://fake.invalid", APIKey: "synthetic", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	finish := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	provider := &wakeSocketProvider{finish: finish}
	service := chat.NewChatService(repo, repo, llm.NewFactory(provider), nil, nil)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		service.CloseBackground(ctx)
	}()
	controller := NewController(service, nil)
	server := httptest.NewServer(http.HandlerFunc(controller.Chat))
	defer server.Close()
	connect := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		return conn
	}
	read := func(conn *websocket.Conn) map[string]any {
		t.Helper()
		var message map[string]any
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}
		return message
	}
	conn := connect()
	if err = conn.WriteJSON(map[string]any{"type": "chat", "sessionId": session.ID, "modelId": model.ID, "content": "运行后台测试"}); err != nil {
		t.Fatal(err)
	}
	for {
		event := read(conn)
		if event["type"] == "error" {
			t.Fatal(event)
		}
		if event["type"] == "done" {
			break
		}
	}
	_ = conn.Close() // The original turn and browser connection have both ended.
	once.Do(func() { close(finish) })
	reconnected := connect()
	defer reconnected.Close()
	if err = reconnected.WriteJSON(map[string]any{"type": "watch_session", "sessionId": session.ID}); err != nil {
		t.Fatal(err)
	}
	for {
		event := read(reconnected)
		if event["type"] == "error" {
			t.Fatal(event)
		}
		if event["toolCallId"] == "wake-needs-approval" && event["requiresApproval"] == true {
			break
		}
	}
	if err = reconnected.WriteJSON(map[string]any{"type": "tool_approve", "sessionId": session.ID, "toolCallId": "wake-needs-approval", "approved": false}); err != nil {
		t.Fatal(err)
	}
	rejected := false
	for {
		event := read(reconnected)
		if event["type"] == "error" {
			t.Fatal(event)
		}
		if event["toolCallId"] == "wake-needs-approval" && event["status"] == "rejected" {
			rejected = true
		}
		if event["type"] == "done" {
			if !strings.Contains(event["answer"].(string), "后续操作已拒绝") {
				t.Fatal(event)
			}
			break
		}
	}
	if !rejected {
		t.Fatal("background continuation bypassed approval")
	}
	messages, err := repo.ListAllSessionMessages(ctx, session.ID, 20)
	if err != nil || len(messages) != 3 {
		t.Fatalf("history %d: %v", len(messages), err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.calls != 3 {
		t.Fatalf("duplicate continuation: %d provider calls", provider.calls)
	}
}
