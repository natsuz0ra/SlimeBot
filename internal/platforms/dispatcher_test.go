package platforms

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/internal/constants"
	chatsvc "slimebot/internal/services/chat"
)

type mockPlatformChatService struct{}

func (m *mockPlatformChatService) EnsureMessagePlatformSession(_ context.Context, _ string) (*domain.Session, error) {
	return &domain.Session{ID: constants.MessagePlatformSessionID, Name: constants.MessagePlatformSessionName}, nil
}

func (m *mockPlatformChatService) ResolvePlatformModel(_ context.Context) (string, error) {
	return "mock-model-id", nil
}

func (m *mockPlatformChatService) ResolvePlatformRuntimeSettings(_ context.Context) (string, string, error) {
	return "off", constants.ApprovalModeStandard, nil
}

func (m *mockPlatformChatService) RunTurn(_ context.Context, _ chatsvc.ChatTurnRequest, callbacks chatsvc.AgentCallbacks) (*chatsvc.ChatStreamResult, error) {
	_ = callbacks.OnToolCallStart(chatsvc.ApprovalRequest{
		ToolCallID:       "tc_1",
		ToolName:         "http_request",
		Command:          "run",
		Params:           map[string]any{"cmd": "echo hi"},
		RequiresApproval: false,
	})
	_ = callbacks.OnToolCallResult(chatsvc.ToolCallResult{
		ToolCallID:       "tc_1",
		ToolName:         "http_request",
		Command:          "run",
		RequiresApproval: false,
		Status:           "completed",
	})
	return &chatsvc.ChatStreamResult{Answer: "hello-from-mock"}, nil
}

type captureAttachmentChatService struct {
	lastPlatform string
	lastRequest  chatsvc.ChatTurnRequest
}

func (m *captureAttachmentChatService) EnsureMessagePlatformSession(_ context.Context, platform string) (*domain.Session, error) {
	m.lastPlatform = platform
	return &domain.Session{ID: constants.MessagePlatformSessionIDFor(platform), Name: platform}, nil
}

func (m *captureAttachmentChatService) ResolvePlatformModel(_ context.Context) (string, error) {
	return "mock-model-id", nil
}

func (m *captureAttachmentChatService) ResolvePlatformRuntimeSettings(_ context.Context) (string, string, error) {
	return "high", constants.ApprovalModeAutoReview, nil
}

func (m *captureAttachmentChatService) RunTurn(_ context.Context, req chatsvc.ChatTurnRequest, _ chatsvc.AgentCallbacks) (*chatsvc.ChatStreamResult, error) {
	m.lastRequest = req
	return &chatsvc.ChatStreamResult{Answer: "ok"}, nil
}

type mockApprovalChatService struct{}

func (m *mockApprovalChatService) EnsureMessagePlatformSession(_ context.Context, _ string) (*domain.Session, error) {
	return &domain.Session{ID: constants.MessagePlatformSessionID, Name: constants.MessagePlatformSessionName}, nil
}

func (m *mockApprovalChatService) ResolvePlatformModel(_ context.Context) (string, error) {
	return "mock-model-id", nil
}

func (m *mockApprovalChatService) ResolvePlatformRuntimeSettings(_ context.Context) (string, string, error) {
	return "off", constants.ApprovalModeStandard, nil
}

func (m *mockApprovalChatService) RunTurn(ctx context.Context, _ chatsvc.ChatTurnRequest, callbacks chatsvc.AgentCallbacks) (*chatsvc.ChatStreamResult, error) {
	_ = callbacks.OnToolCallStart(chatsvc.ApprovalRequest{
		ToolCallID:       "tc_approval",
		ToolName:         "exec",
		Command:          "run",
		Params:           map[string]any{"cmd": "echo hi"},
		RequiresApproval: true,
	})
	approval, err := callbacks.WaitApproval(ctx, "tc_approval")
	if err != nil {
		return nil, err
	}
	status := constants.ToolCallStatusRejected
	errText := "Execution was rejected by the user."
	if approval != nil && approval.Approved {
		status = constants.ToolCallStatusCompleted
		errText = ""
	}
	_ = callbacks.OnToolCallResult(chatsvc.ToolCallResult{
		ToolCallID:       "tc_approval",
		ToolName:         "exec",
		Command:          "run",
		RequiresApproval: true,
		Status:           status,
		Error:            errText,
	})
	return &chatsvc.ChatStreamResult{Answer: "approval-flow-done"}, nil
}

type mockSender struct {
	items       []string
	approveData string
}

type mockApprovalEntry struct {
	toolCallID string
	chatID     string
	approved   bool
}

type mockApprovalBroker struct {
	mu         sync.Mutex
	waitByTool map[string]chan chatsvc.ApprovalResponse
	byToken    map[string]mockApprovalEntry
	waiting    chan struct{}
}

func newMockApprovalBroker() *mockApprovalBroker {
	return &mockApprovalBroker{
		waitByTool: make(map[string]chan chatsvc.ApprovalResponse),
		byToken:    make(map[string]mockApprovalEntry),
	}
}

func (m *mockApprovalBroker) Register(toolCallID string, chatID string, _ time.Duration) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(toolCallID) == "" || strings.TrimSpace(chatID) == "" {
		return "", "", fmt.Errorf("toolCallID/chatID is required")
	}
	waitCh := make(chan chatsvc.ApprovalResponse, 1)
	m.waitByTool[toolCallID] = waitCh

	approveToken := "ap_token_" + toolCallID
	rejectToken := "rj_token_" + toolCallID
	m.byToken[approveToken] = mockApprovalEntry{toolCallID: toolCallID, chatID: chatID, approved: true}
	m.byToken[rejectToken] = mockApprovalEntry{toolCallID: toolCallID, chatID: chatID, approved: false}
	return "ap:" + approveToken, "rj:" + rejectToken, nil
}

func (m *mockApprovalBroker) Wait(ctx context.Context, toolCallID string) (*chatsvc.ApprovalResponse, error) {
	m.mu.Lock()
	ch, ok := m.waitByTool[toolCallID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("approval context not found")
	}
	if m.waiting != nil {
		close(m.waiting)
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
	var waitCh chan chatsvc.ApprovalResponse
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
	resp := chatsvc.ApprovalResponse{ToolCallID: entry.toolCallID, Approved: entry.approved}
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

func (m *mockSender) SendApprovalKeyboard(_ string, text string, approveData string, _ string) error {
	m.items = append(m.items, text)
	m.approveData = approveData
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
	start := sender.items[0]
	if !strings.Contains(start, "run") {
		t.Fatalf("expected tool start message to include command, got=%s", start)
	}
	last := sender.items[len(sender.items)-1]
	if last != "hello-from-mock" {
		t.Fatalf("expected final answer message, got=%s", last)
	}
}

func TestFormatToolStartSummary_WithoutCommand(t *testing.T) {
	got := formatToolStartSummary(chatsvc.ApprovalRequest{
		ToolName: "http_request",
		Command:  "",
		Params:   map[string]any{},
	})
	if got != "Tool execution started: http_request" {
		t.Fatalf("unexpected start summary without command, got=%s", got)
	}
}

func TestFormatToolApprovalPrompt_ShowsExecCommandParam(t *testing.T) {
	got := formatToolApprovalPrompt(chatsvc.ApprovalRequest{
		ToolName: "exec",
		Command:  "run",
		Params:   map[string]any{"command": "go test ./..."},
	})
	want := "Tool execution requires approval: exec\nCommand: go test ./...\nPlease choose Approve or Reject."
	if got != want {
		t.Fatalf("unexpected approval prompt:\nwant=%q\n got=%q", want, got)
	}
}

func TestFormatToolApprovalPrompt_TruncatesLongExecCommandByRunes(t *testing.T) {
	longCommand := strings.Repeat("测", telegramApprovalCommandMaxRunes+1)
	got := formatToolApprovalPrompt(chatsvc.ApprovalRequest{
		ToolName: "exec",
		Command:  "run",
		Params:   map[string]any{"command": longCommand},
	})
	commandLine := "Command: " + strings.Repeat("测", telegramApprovalCommandMaxRunes) + "..."
	if !strings.Contains(got, commandLine) {
		t.Fatalf("expected truncated rune-safe command line, got=%q", got)
	}
}

func TestFormatToolApprovalPrompt_OmitsEmptyCommandLine(t *testing.T) {
	got := formatToolApprovalPrompt(chatsvc.ApprovalRequest{
		ToolName: "exec",
		Command:  "",
		Params:   map[string]any{"command": "   "},
	})
	if strings.Contains(got, "Command:") {
		t.Fatalf("expected empty command line to be omitted, got=%q", got)
	}
}

func TestDispatcherHandleInbound_ApprovalByCallback(t *testing.T) {
	broker := newMockApprovalBroker()
	broker.waiting = make(chan struct{})
	dispatcher := NewDispatcher(&mockApprovalChatService{}, broker)
	sender := &mockSender{}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- dispatcher.HandleInbound(ctx, InboundMessage{
			Platform: constants.TelegramPlatformName,
			ChatID:   "20001",
			Text:     "hello",
		}, sender)
	}()

	select {
	case <-broker.waiting:
	case <-ctx.Done():
		t.Fatal("dispatcher did not start waiting for approval")
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

func TestDispatcherHandleInbound_PassesAttachmentIDs(t *testing.T) {
	chatSvc := &captureAttachmentChatService{}
	dispatcher := NewDispatcher(chatSvc, newMockApprovalBroker())
	sender := &mockSender{}

	err := dispatcher.HandleInbound(context.Background(), InboundMessage{
		Platform:      constants.TelegramPlatformName,
		ChatID:        "30001",
		Text:          "",
		AttachmentIDs: []string{" att_a ", "", "att_b"},
	}, sender)
	if err != nil {
		t.Fatalf("handle inbound failed: %v", err)
	}
	if len(chatSvc.lastRequest.AttachmentIDs) != 2 {
		t.Fatalf("expected 2 attachment ids, got=%d", len(chatSvc.lastRequest.AttachmentIDs))
	}
	if chatSvc.lastRequest.AttachmentIDs[0] != "att_a" || chatSvc.lastRequest.AttachmentIDs[1] != "att_b" {
		t.Fatalf("unexpected attachment ids: %#v", chatSvc.lastRequest.AttachmentIDs)
	}
	if chatSvc.lastRequest.Content != "" {
		t.Fatalf("expected empty content for attachment-only input, got=%q", chatSvc.lastRequest.Content)
	}
}

func TestDispatcherHandleInbound_PassesPlatformRuntimeSettings(t *testing.T) {
	chatSvc := &captureAttachmentChatService{}
	dispatcher := NewDispatcher(chatSvc, newMockApprovalBroker())
	sender := &mockSender{}

	err := dispatcher.HandleInbound(context.Background(), InboundMessage{
		Platform: constants.TelegramPlatformName,
		ChatID:   "30002",
		Text:     "hello",
	}, sender)
	if err != nil {
		t.Fatalf("handle inbound failed: %v", err)
	}

	if chatSvc.lastRequest.ThinkingLevel != "high" {
		t.Fatalf("thinking level = %q, want high", chatSvc.lastRequest.ThinkingLevel)
	}
	if chatSvc.lastRequest.ApprovalModeOverride != constants.ApprovalModeAutoReview {
		t.Fatalf("approval mode = %q, want %q", chatSvc.lastRequest.ApprovalModeOverride, constants.ApprovalModeAutoReview)
	}
	if chatSvc.lastRequest.RequestID == "" || chatSvc.lastRequest.ModelID != "mock-model-id" || chatSvc.lastRequest.Content != "hello" {
		t.Fatalf("inbound turn = %+v", chatSvc.lastRequest)
	}
}

func TestDispatcherHandleInbound_UsesPlatformSession(t *testing.T) {
	chatSvc := &captureAttachmentChatService{}
	dispatcher := NewDispatcher(chatSvc, newMockApprovalBroker())
	err := dispatcher.HandleInbound(context.Background(), InboundMessage{
		Platform: "discord",
		ChatID:   "30003",
		Text:     "hello",
	}, &mockSender{})
	if err != nil {
		t.Fatalf("handle inbound failed: %v", err)
	}
	if chatSvc.lastPlatform != "discord" || chatSvc.lastRequest.SessionID != constants.MessagePlatformSessionIDFor("discord") {
		t.Fatalf("platform=%q session=%q", chatSvc.lastPlatform, chatSvc.lastRequest.SessionID)
	}
}

type markerAnswerChatService struct {
	answer string
}

func (m *markerAnswerChatService) EnsureMessagePlatformSession(_ context.Context, _ string) (*domain.Session, error) {
	return &domain.Session{ID: constants.MessagePlatformSessionID, Name: constants.MessagePlatformSessionName}, nil
}

func (m *markerAnswerChatService) ResolvePlatformModel(_ context.Context) (string, error) {
	return "mock-model-id", nil
}

func (m *markerAnswerChatService) ResolvePlatformRuntimeSettings(_ context.Context) (string, string, error) {
	return "off", constants.ApprovalModeStandard, nil
}

func (m *markerAnswerChatService) RunTurn(_ context.Context, _ chatsvc.ChatTurnRequest, _ chatsvc.AgentCallbacks) (*chatsvc.ChatStreamResult, error) {
	return &chatsvc.ChatStreamResult{Answer: m.answer}, nil
}

func TestDispatcherHandleInbound_SplitsFinalAnswerAroundMarkers(t *testing.T) {
	dispatcher := NewDispatcher(&markerAnswerChatService{answer: "First paragraph.\n<!-- TOOL_CALL:tc_1 -->\nSecond paragraph."}, newMockApprovalBroker())
	sender := &mockSender{}

	err := dispatcher.HandleInbound(context.Background(), InboundMessage{
		Platform: constants.TelegramPlatformName,
		ChatID:   "40001",
		Text:     "hello",
	}, sender)
	if err != nil {
		t.Fatalf("handle inbound failed: %v", err)
	}

	if len(sender.items) != 2 {
		t.Fatalf("expected 2 final answer chunks, got %d: %#v", len(sender.items), sender.items)
	}
	if sender.items[0] != "First paragraph." || sender.items[1] != "Second paragraph." {
		t.Fatalf("unexpected final answer chunks: %#v", sender.items)
	}
}

func TestDispatcherHandleInbound_DoesNotSendMarkerOnlyFinalAnswer(t *testing.T) {
	dispatcher := NewDispatcher(&markerAnswerChatService{answer: "\n<!-- TOOL_CALL:tc_1 -->\n<!-- THINKING:th_1 -->\n"}, newMockApprovalBroker())
	sender := &mockSender{}

	err := dispatcher.HandleInbound(context.Background(), InboundMessage{
		Platform: constants.TelegramPlatformName,
		ChatID:   "40002",
		Text:     "hello",
	}, sender)
	if err != nil {
		t.Fatalf("handle inbound failed: %v", err)
	}

	if len(sender.items) != 0 {
		t.Fatalf("expected no messages for marker-only final answer, got %#v", sender.items)
	}
}

func TestSplitTelegramTextForSend_LimitsLongMessages(t *testing.T) {
	longLine := strings.Repeat("x", telegramMessageChunkLimit+25)
	chunks := splitTelegramTextForSend("intro\n\n" + longLine)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %#v", chunks)
	}
	for _, chunk := range chunks {
		if len(chunk) > telegramMessageChunkLimit {
			t.Fatalf("chunk length = %d, limit = %d", len(chunk), telegramMessageChunkLimit)
		}
		if strings.TrimSpace(chunk) == "" {
			t.Fatalf("unexpected blank chunk in %#v", chunks)
		}
	}
}
