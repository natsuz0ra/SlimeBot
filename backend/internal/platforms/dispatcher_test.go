package platforms

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/services"
)

type mockPlatformChatService struct{}

func (m *mockPlatformChatService) EnsureMessagePlatformSession() (*models.Session, error) {
	return &models.Session{ID: consts.MessagePlatformSessionID, Name: consts.MessagePlatformSessionName}, nil
}

func (m *mockPlatformChatService) ResolvePlatformModel() (string, error) {
	return "mock-model-id", nil
}

func (m *mockPlatformChatService) HandleChatStream(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	callbacks services.AgentCallbacks,
) (*services.ChatStreamResult, error) {
	_ = callbacks.OnToolCallStart(services.ApprovalRequest{
		ToolCallID:       "tc_1",
		ToolName:         "http_request",
		Command:          "run",
		Params:           map[string]string{"cmd": "echo hi"},
		RequiresApproval: false,
	})
	_ = callbacks.OnToolCallResult(services.ToolCallResult{
		ToolCallID:       "tc_1",
		ToolName:         "http_request",
		Command:          "run",
		RequiresApproval: false,
		Status:           "completed",
	})
	return &services.ChatStreamResult{Answer: "hello-from-mock"}, nil
}

type mockApprovalChatService struct{}

func (m *mockApprovalChatService) EnsureMessagePlatformSession() (*models.Session, error) {
	return &models.Session{ID: consts.MessagePlatformSessionID, Name: consts.MessagePlatformSessionName}, nil
}

func (m *mockApprovalChatService) ResolvePlatformModel() (string, error) {
	return "mock-model-id", nil
}

func (m *mockApprovalChatService) HandleChatStream(
	ctx context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	callbacks services.AgentCallbacks,
) (*services.ChatStreamResult, error) {
	_ = callbacks.OnToolCallStart(services.ApprovalRequest{
		ToolCallID:       "tc_approval",
		ToolName:         "exec",
		Command:          "run",
		Params:           map[string]string{"cmd": "echo hi"},
		RequiresApproval: true,
	})
	approval, err := callbacks.WaitApproval(ctx, "tc_approval")
	if err != nil {
		return nil, err
	}
	status := consts.ToolCallStatusRejected
	errText := "Execution was rejected by the user."
	if approval != nil && approval.Approved {
		status = consts.ToolCallStatusCompleted
		errText = ""
	}
	_ = callbacks.OnToolCallResult(services.ToolCallResult{
		ToolCallID:       "tc_approval",
		ToolName:         "exec",
		Command:          "run",
		RequiresApproval: true,
		Status:           status,
		Error:            errText,
	})
	return &services.ChatStreamResult{Answer: "approval-flow-done"}, nil
}

type mockSender struct {
	items       []string
	approveData string
	rejectData  string
}

type mockApprovalEntry struct {
	toolCallID string
	chatID     string
	approved   bool
}

type mockApprovalBroker struct {
	mu         sync.Mutex
	waitByTool map[string]chan services.ApprovalResponse
	byToken    map[string]mockApprovalEntry
}

func newMockApprovalBroker() *mockApprovalBroker {
	return &mockApprovalBroker{
		waitByTool: make(map[string]chan services.ApprovalResponse),
		byToken:    make(map[string]mockApprovalEntry),
	}
}

func (m *mockApprovalBroker) Register(toolCallID string, chatID string, _ time.Duration) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(toolCallID) == "" || strings.TrimSpace(chatID) == "" {
		return "", "", fmt.Errorf("toolCallID/chatID is required")
	}
	waitCh := make(chan services.ApprovalResponse, 1)
	m.waitByTool[toolCallID] = waitCh

	approveToken := "ap_token_" + toolCallID
	rejectToken := "rj_token_" + toolCallID
	m.byToken[approveToken] = mockApprovalEntry{toolCallID: toolCallID, chatID: chatID, approved: true}
	m.byToken[rejectToken] = mockApprovalEntry{toolCallID: toolCallID, chatID: chatID, approved: false}
	return "ap:" + approveToken, "rj:" + rejectToken, nil
}

func (m *mockApprovalBroker) Wait(ctx context.Context, toolCallID string) (*services.ApprovalResponse, error) {
	m.mu.Lock()
	ch, ok := m.waitByTool[toolCallID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("approval context not found")
	}

	select {
	case <-ctx.Done():
		m.Remove(toolCallID)
		return nil, ctx.Err()
	case resp := <-ch:
		return &resp, nil
	}
}

func (m *mockApprovalBroker) ResolveByCallback(chatID string, callbackData string) (bool, error) {
	parts := strings.SplitN(strings.TrimSpace(callbackData), ":", 2)
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid callback data")
	}
	token := strings.TrimSpace(parts[1])
	m.mu.Lock()
	entry, ok := m.byToken[token]
	var waitCh chan services.ApprovalResponse
	if ok {
		waitCh = m.waitByTool[entry.toolCallID]
		delete(m.byToken, token)
		delete(m.waitByTool, entry.toolCallID)
	}
	m.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("approval token is invalid or expired")
	}
	if strings.TrimSpace(chatID) != entry.chatID {
		return false, fmt.Errorf("approval token does not belong to this chat")
	}
	resp := services.ApprovalResponse{ToolCallID: entry.toolCallID, Approved: entry.approved}
	select {
	case waitCh <- resp:
	default:
	}
	return entry.approved, nil
}

func (m *mockApprovalBroker) Remove(toolCallID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.waitByTool, toolCallID)
	for token, entry := range m.byToken {
		if entry.toolCallID == toolCallID {
			delete(m.byToken, token)
		}
	}
}

func (m *mockSender) SendText(_ string, text string) error {
	m.items = append(m.items, text)
	return nil
}

func (m *mockSender) SendApprovalKeyboard(_ string, text string, approveData string, rejectData string) error {
	m.items = append(m.items, text)
	m.approveData = approveData
	m.rejectData = rejectData
	return nil
}

func TestDispatcherHandleInbound_SendsToolSummaryAndFinalAnswer(t *testing.T) {
	dispatcher := NewDispatcher(&mockPlatformChatService{}, newMockApprovalBroker())
	sender := &mockSender{}

	err := dispatcher.HandleInbound(context.Background(), InboundMessage{
		Platform: "telegram",
		ChatID:   "10001",
		Text:     "hello",
	}, sender)
	if err != nil {
		t.Fatalf("handle inbound failed: %v", err)
	}
	if len(sender.items) < 3 {
		t.Fatalf("expected at least 3 messages(tool start, tool result, final), got=%d", len(sender.items))
	}
	last := sender.items[len(sender.items)-1]
	if last != "hello-from-mock" {
		t.Fatalf("expected final answer message, got=%s", last)
	}
}

func TestDispatcherHandleInbound_ApprovalByCallback(t *testing.T) {
	dispatcher := NewDispatcher(&mockApprovalChatService{}, newMockApprovalBroker())
	sender := &mockSender{}

	done := make(chan error, 1)
	go func() {
		done <- dispatcher.HandleInbound(context.Background(), InboundMessage{
			Platform: consts.TelegramPlatformName,
			ChatID:   "20001",
			Text:     "hello",
		}, sender)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for strings.TrimSpace(sender.approveData) == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if strings.TrimSpace(sender.approveData) == "" {
		t.Fatalf("approve callback data was not sent")
	}

	approved, err := dispatcher.HandleTelegramApprovalCallback("20001", sender.approveData)
	if err != nil {
		t.Fatalf("resolve approval callback failed: %v", err)
	}
	if !approved {
		t.Fatalf("expected approved=true")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("handle inbound failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handle inbound timed out after approval")
	}
}
