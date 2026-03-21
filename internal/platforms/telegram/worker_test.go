package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/platforms"
	chatsvc "slimebot/internal/services/chat"
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

func (m *mockBrokerForWorker) Wait(context.Context, string) (*chatsvc.ApprovalResponse, error) {
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

type mockUploadService struct {
	items []chatsvc.UploadedAttachment
}

func (m *mockUploadService) RegisterLocalFiles(_ string, files []chatsvc.LocalAttachmentFile) ([]chatsvc.UploadedAttachment, error) {
	items := make([]chatsvc.UploadedAttachment, 0, len(files))
	for i, file := range files {
		items = append(items, chatsvc.UploadedAttachment{
			ID:        "att_" + strconv.Itoa(i+1),
			SessionID: constants.MessagePlatformSessionID,
			Name:      file.Name,
			SizeBytes: int64(len(file.Data)),
			MimeType:  strings.TrimSpace(file.MimeType),
			Category:  "document",
		})
	}
	m.items = items
	return items, nil
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

func TestProcessUpdates_UsesCaptionWhenTextEmpty(t *testing.T) {
	dispatched := make(chan platforms.InboundMessage, 1)
	w := &Worker{
		dispatchSlots: make(chan struct{}, 1),
	}
	w.dispatchInbound = func(_ context.Context, inbound platforms.InboundMessage, _ platforms.OutboundSender) error {
		dispatched <- inbound
		return nil
	}
	adapter := &Adapter{token: "dummy", http: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	})}}

	w.processUpdates(context.Background(), adapter, []update{
		{
			UpdateID: 1,
			Message: &message{
				Chat:    chat{ID: 10001},
				Caption: "  from-caption  ",
			},
		},
	}, 0)

	select {
	case inbound := <-dispatched:
		if inbound.Text != "from-caption" {
			t.Fatalf("expected caption fallback text, got=%q", inbound.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("expected inbound dispatch")
	}
}

func TestProcessUpdates_MediaMessageBuildsAttachmentIDs(t *testing.T) {
	dispatched := make(chan platforms.InboundMessage, 1)
	w := &Worker{
		dispatchSlots: make(chan struct{}, 1),
		uploads:       &mockUploadService{},
	}
	w.dispatchInbound = func(_ context.Context, inbound platforms.InboundMessage, _ platforms.OutboundSender) error {
		dispatched <- inbound
		return nil
	}

	adapter := &Adapter{
		token: "dummy-token",
		http: &http.Client{
			Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.String(), "/getFile") {
					query, _ := url.ParseQuery(req.URL.RawQuery)
					if strings.TrimSpace(query.Get("file_id")) != "file-doc-1" {
						return nil, errors.New("unexpected file_id")
					}
					payload := map[string]any{"ok": true, "result": map[string]any{"file_path": "documents/doc1.txt"}}
					raw, _ := json.Marshal(payload)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: make(http.Header)}, nil
				}
				if strings.Contains(req.URL.String(), "/file/bot") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString("hello attachment")),
						Header:     make(http.Header),
					}, nil
				}
				return nil, errors.New("unexpected request")
			}),
		},
	}

	w.processUpdates(context.Background(), adapter, []update{
		{
			UpdateID: 1,
			Message: &message{
				Chat: chat{ID: 10002},
				Document: &docAttachment{
					FileID:   "file-doc-1",
					FileName: "doc1.txt",
					MimeType: "text/plain",
				},
			},
		},
	}, 0)

	select {
	case inbound := <-dispatched:
		if len(inbound.AttachmentIDs) != 1 {
			t.Fatalf("expected 1 attachment id, got=%d", len(inbound.AttachmentIDs))
		}
		if len(inbound.Attachments) != 1 {
			t.Fatalf("expected 1 attachment metadata, got=%d", len(inbound.Attachments))
		}
		if inbound.Attachments[0].Source != "document" {
			t.Fatalf("expected document source, got=%s", inbound.Attachments[0].Source)
		}
	case <-time.After(time.Second):
		t.Fatal("expected inbound dispatch")
	}
}
