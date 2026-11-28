package ws

import (
	"context"
	"slices"
	"testing"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	chatsvc "slimebot/internal/services/chat"
	llmsvc "slimebot/internal/services/llm"
)

type wsTurnProvider struct{}

func (wsTurnProvider) StreamChatWithTools(_ context.Context, _ llmsvc.ModelRuntimeConfig, messages []llmsvc.ChatMessage, _ []llmsvc.ToolDef, cb llmsvc.StreamCallbacks) (*llmsvc.StreamResult, error) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			if err := cb.OnChunk("answer: " + messages[i].Content); err != nil {
				return nil, err
			}
			break
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

func TestWSAdaptsChatAndEditToRecordedTurns(t *testing.T) {
	ctx := context.Background()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "ws_chat_turns"))
	t.Cleanup(func() { _ = repo.Close() })
	session, err := repo.CreateSession(ctx, "WebSocket turns")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{
		Name: "fake", Provider: llmsvc.ProviderOpenAI, BaseURL: "http://fake.invalid", APIKey: "fake", Model: "fake",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := chatsvc.NewChatService(repo, nil, llmsvc.NewFactory(wsTurnProvider{}), nil, nil)
	controller := NewController(service, nil)
	broker, active := newApprovalBroker(), &activeChatCanceler{}

	run := func(incoming chatIncoming, wantEvents []string) (map[string]map[string]any, []domain.Message) {
		t.Helper()
		var order []string
		events := make(map[string]map[string]any)
		if !controller.handleChatIncoming(ctx, func(payload any) bool {
			if event, ok := payload.(map[string]any); ok {
				kind, _ := event["type"].(string)
				switch kind {
				case "session", "start", "message_edited", "done", "error":
					order = append(order, kind)
					events[kind] = event
				}
			}
			return true
		}, broker, active, incoming) {
			t.Fatal("controller stopped")
		}
		if !slices.Equal(order, wantEvents) {
			t.Fatalf("events = %v, want %v (%+v)", order, wantEvents, events)
		}
		items, err := repo.ListAllSessionMessages(ctx, session.ID, -1)
		if err != nil || len(items) != 2 {
			t.Fatalf("turn history = %+v, %v", items, err)
		}
		requestID, _ := events["start"]["turnId"].(string)
		if requestID == "" || items[1].RequestID != requestID || events["done"]["answer"] != items[1].Content {
			t.Fatalf("turn identity or answer mismatch: events=%+v history=%+v", events, items)
		}
		return events, items
	}

	first, initial := run(chatIncoming{
		Type: "chat", SessionID: session.ID, ModelID: model.ID, Content: "internal prompt", DisplayContent: "visible prompt",
	}, []string{"session", "start", "done"})
	start, err := time.Parse(time.RFC3339Nano, first["start"]["startedAt"].(string))
	if err != nil || !start.Equal(initial[0].CreatedAt) || initial[0].Content != "visible prompt" || initial[1].Content != "answer: internal prompt" {
		t.Fatalf("initial turn = %+v, start=%v (%v)", initial, start, err)
	}

	edited, after := run(chatIncoming{
		Type: "chat_edit", SessionID: session.ID, ModelID: model.ID, MessageID: initial[0].ID, Content: "edited prompt",
	}, []string{"session", "message_edited", "start", "done"})
	edit := edited["message_edited"]
	editAt, err := time.Parse(time.RFC3339Nano, edit["createdAt"].(string))
	if edit["messageId"] != initial[0].ID || edit["content"] != "edited prompt" || after[0].ID != initial[0].ID ||
		err != nil || !after[0].CreatedAt.Equal(editAt) || !after[0].CreatedAt.After(initial[0].CreatedAt) || after[0].Content != "edited prompt" ||
		after[1].ID == initial[1].ID || after[1].RequestID == initial[1].RequestID || after[1].Content != "answer: edited prompt" {
		t.Fatalf("edited event = %+v, edited history = %+v, initial history = %+v", edit, after, initial)
	}
}
