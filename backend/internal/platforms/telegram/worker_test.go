package telegram

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/backend/internal/platforms"
	"slimebot/backend/internal/services"
)

type mockWorkerSender struct {
	mu    sync.Mutex
	texts []string
}

func (m *mockWorkerSender) SendText(_ string, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return nil
}

func (m *mockWorkerSender) SendApprovalKeyboard(_ string, _ string, _ string, _ string) error {
	return nil
}

func (m *mockWorkerSender) Texts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]string, len(m.texts))
	copy(items, m.texts)
	return items
}

func TestDispatchInboundAsync_NonBlocking(t *testing.T) {
	w := &Worker{
		dispatchSlots: make(chan struct{}, 1),
	}
	block := make(chan struct{})
	done := make(chan struct{})
	w.dispatchInbound = func(_ context.Context, _ platforms.InboundMessage, _ platforms.OutboundSender) error {
		defer close(done)
		<-block
		return nil
	}

	sender := &mockWorkerSender{}
	start := time.Now()
	w.dispatchInboundAsync(context.Background(), platforms.InboundMessage{ChatID: "chat_1", Text: "hello"}, sender)
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("dispatchInboundAsync should return quickly")
	}

	close(block)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async dispatch did not finish")
	}
}

func TestDispatchInboundAsync_ConcurrencyGuard(t *testing.T) {
	w := &Worker{
		dispatchSlots: make(chan struct{}, 1),
	}
	block := make(chan struct{})
	w.dispatchInbound = func(_ context.Context, _ platforms.InboundMessage, _ platforms.OutboundSender) error {
		<-block
		return nil
	}

	sender := &mockWorkerSender{}
	w.dispatchInboundAsync(context.Background(), platforms.InboundMessage{ChatID: "chat_busy", Text: "first"}, sender)
	w.dispatchInboundAsync(context.Background(), platforms.InboundMessage{ChatID: "chat_busy", Text: "second"}, sender)
	close(block)

	time.Sleep(30 * time.Millisecond)
	texts := sender.Texts()
	if len(texts) == 0 {
		t.Fatal("expected busy message when dispatch slots are full")
	}
	if !strings.Contains(texts[0], "System is busy") {
		t.Fatalf("unexpected busy message: %s", texts[0])
	}
}

type mockBrokerForWorker struct{}

func (m *mockBrokerForWorker) Register(string, string, time.Duration) (string, string, error) {
	return "", "", nil
}

func (m *mockBrokerForWorker) Wait(context.Context, string) (*services.ApprovalResponse, error) {
	return nil, nil
}

func (m *mockBrokerForWorker) ResolveByCallback(string, string) (bool, error) {
	return false, errors.New("invalid token")
}

func (m *mockBrokerForWorker) Remove(string) {}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHandleApprovalCallback_InvalidDataAnswerError(t *testing.T) {
	dispatcher := platforms.NewDispatcher(nil, &mockBrokerForWorker{})
	w := &Worker{dispatcher: dispatcher}

	var calledURL string
	var calledBody []byte
	adapter := &Adapter{
		token: "dummy-token",
		http: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calledURL = req.URL.String()
				body, _ := io.ReadAll(req.Body)
				calledBody = body
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"ok":true}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	w.handleApprovalCallback(&callbackQuery{
		ID:   "cb_1",
		Data: "ap:invalid",
		Message: &message{
			Chat: chat{ID: 10001},
		},
	}, adapter)

	if !strings.Contains(calledURL, "answerCallbackQuery") {
		t.Fatalf("expected answerCallbackQuery call, got=%s", calledURL)
	}
	if !strings.Contains(string(calledBody), "Approval failed") {
		t.Fatalf("expected approval failure hint in callback answer body, got=%s", string(calledBody))
	}
}
