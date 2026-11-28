package chat

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	llmsvc "slimebot/internal/services/llm"
)

type recordedTurnProvider struct {
	calls          int
	cancelOnAnswer bool
}

func (p *recordedTurnProvider) StreamChatWithTools(ctx context.Context, _ llmsvc.ModelRuntimeConfig, _ []llmsvc.ChatMessage, _ []llmsvc.ToolDef, cb llmsvc.StreamCallbacks) (*llmsvc.StreamResult, error) {
	p.calls++
	if p.calls == 1 {
		if err := cb.OnThinkingChunk("first thought"); err != nil {
			return nil, err
		}
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultToolCalls, ToolCalls: []llmsvc.ToolCallInfo{{
			ID: "list-todos", Name: "todo__list", Arguments: "{}",
		}}}, nil
	}
	if p.cancelOnAnswer {
		if err := cb.OnThinkingChunk("interrupted thought"); err != nil {
			return nil, err
		}
		return nil, ctx.Err()
	}
	if err := cb.OnChunk("complete answer"); err != nil {
		return nil, err
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

func turnTestRequest(t *testing.T, repo *repositories.Repository) ChatTurnRequest {
	t.Helper()
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "Recorded turn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{
		Name: "fake", Provider: llmsvc.ProviderOpenAI, BaseURL: "http://fake.invalid", APIKey: "fake", Model: "fake",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ChatTurnRequest{
		SessionID: session.ID, RequestID: "recorded-turn", Content: "inspect todos", ModelID: model.ID,
		ThinkingLevel: "high", ApprovalModeOverride: constants.ApprovalModeAuto,
	}
}

func TestRunTurnEventSinkFailuresDoNotPreventRecording(t *testing.T) {
	for _, event := range []string{"turn start", "tool start", "tool result", "thinking start", "thinking chunk", "thinking done", "answer"} {
		t.Run(event, func(t *testing.T) {
			ctx := context.Background()
			repo := newTestRepo(t)
			t.Cleanup(func() { _ = repo.Close() })
			req := turnTestRequest(t, repo)
			provider := &recordedTurnProvider{}
			service := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
			pushErr := errors.New("transport unavailable")
			send := func(kind string) error {
				if kind == event {
					return pushErr
				}
				return nil
			}
			result, err := service.RunTurn(ctx, req, AgentCallbacks{
				OnTurnStarted:    func(string) error { return send("turn start") },
				OnToolCallStart:  func(ApprovalRequest) error { return send("tool start") },
				OnToolCallResult: func(ToolCallResult) error { return send("tool result") },
				OnThinkingStart:  func(ThinkingEventMeta) error { return send("thinking start") },
				OnThinkingChunk:  func(string, ThinkingEventMeta) error { return send("thinking chunk") },
				OnThinkingDone:   func(ThinkingEventMeta) error { return send("thinking done") },
				OnChunk:          func(string) error { return send("answer") },
			})
			if err != nil || !result.PushFailed || result.PushError != pushErr.Error() || StripContentMarkers(result.Answer) != "complete answer" {
				t.Fatalf("turn result = %+v, %v", result, err)
			}
			assistant := recordedAssistant(t, repo, req)
			records, err := repo.ListSessionToolCallRecordsByAssistantMessageIDs(ctx, req.SessionID, []string{assistant.ID})
			if err != nil || len(records) != 1 || records[0].Status != constants.ToolCallStatusCompleted || records[0].ToolCallID != "list-todos" {
				t.Fatalf("tool records = %+v, %v", records, err)
			}
			thoughts, err := repo.ListSessionThinkingRecordsByAssistantMessageIDs(ctx, req.SessionID, []string{assistant.ID})
			if err != nil || len(thoughts) != 1 || thoughts[0].Status != "completed" || thoughts[0].Content != "first thought" {
				t.Fatalf("thinking records = %+v, %v", thoughts, err)
			}
		})
	}
}

func TestRunTurnReportsPersistenceFailure(t *testing.T) {
	for _, tc := range []struct {
		operation string
		trigger   string
	}{
		{"tool result", "BEFORE UPDATE OF output ON tool_call_records"},
		{"thinking chunk", "BEFORE UPDATE OF content ON thinking_records"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			db := repositories.NewSQLiteDBTest(t, "turn_recorder_failure")
			repo := repositories.New(db)
			t.Cleanup(func() { _ = repo.Close() })
			req := turnTestRequest(t, repo)
			if err := db.Exec("CREATE TRIGGER fail_record " + tc.trigger + " BEGIN SELECT RAISE(ABORT, 'injected durable write failure'); END").Error; err != nil {
				t.Fatal(err)
			}
			provider := &recordedTurnProvider{}
			service := NewChatService(repo, nil, llmsvc.NewFactory(provider), nil, nil)
			notified := false
			result, err := service.RunTurn(context.Background(), req, AgentCallbacks{
				OnTurnStarted: func(string) error { return errors.New("transport unavailable") },
				OnToolCallResult: func(ToolCallResult) error {
					notified = true
					return nil
				},
			})
			var failure *TurnPersistenceError
			if result != nil || !errors.As(err, &failure) || failure.Operation != tc.operation || !strings.Contains(failure.Err.Error(), "injected durable write failure") {
				t.Fatalf("turn result = %+v, %v", result, err)
			}
			messages, err := repo.ListAllSessionMessages(context.Background(), req.SessionID, -1)
			if err != nil || len(messages) != 1 || messages[0].Role != "user" || provider.calls != 1 || notified {
				t.Fatalf("failed turn: messages=%+v provider calls=%d tool result notified=%v err=%v", messages, provider.calls, notified, err)
			}
		})
	}
}

type turnContextKey struct{}

type completionContextStore struct {
	*repositories.Repository
	ctxErr   error
	deadline time.Duration
	value    any
}

func (s *completionContextStore) CompleteChatTurn(ctx context.Context, input domain.CompleteChatTurnInput) (*domain.CompleteChatTurnResult, error) {
	s.ctxErr = ctx.Err()
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline = time.Until(deadline)
	}
	s.value = ctx.Value(turnContextKey{})
	return s.Repository.CompleteChatTurn(ctx, input)
}

func TestRunTurnCompletesInterruptedRecordsWithBoundedContext(t *testing.T) {
	db, err := repositories.NewSQLite(filepath.Join(t.TempDir(), "interrupted.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	repo := repositories.New(db)
	t.Cleanup(func() { _ = repo.Close() })
	req := turnTestRequest(t, repo)
	if err := repo.UpsertToolCallStart(context.Background(), domain.ToolCallStartRecordInput{
		SessionID: req.SessionID, RequestID: req.RequestID, ToolCallID: "open-tool", Status: constants.ToolCallStatusExecuting,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), turnContextKey{}, "scope"))
	defer cancel()
	store := &completionContextStore{Repository: repo}
	service := NewChatService(store, nil, llmsvc.NewFactory(&recordedTurnProvider{cancelOnAnswer: true}), nil, nil)
	result, err := service.RunTurn(ctx, req, AgentCallbacks{
		OnThinkingChunk: func(chunk string, _ ThinkingEventMeta) error {
			if chunk == "interrupted thought" {
				cancel()
			}
			return nil
		},
	})
	if err != nil || !result.IsInterrupted || result.PushFailed || ctx.Err() != context.Canceled {
		t.Fatalf("interrupted turn = %+v, %v, original context = %v", result, err, ctx.Err())
	}
	if store.ctxErr != nil || store.deadline <= 0 || store.deadline > turnFinalizeTimeout || store.value != "scope" {
		t.Fatalf("completion context: err=%v deadline=%v value=%v", store.ctxErr, store.deadline, store.value)
	}
	assistant := recordedAssistant(t, repo, req)
	readCtx := context.Background()
	records, err := repo.ListSessionToolCallRecordsByAssistantMessageIDs(readCtx, req.SessionID, []string{assistant.ID})
	if err != nil || len(records) != 2 {
		t.Fatalf("tool records = %+v, %v", records, err)
	}
	for _, tool := range records {
		if tool.ToolCallID == "open-tool" && (tool.Status != constants.ToolCallStatusError || tool.FinishedAt == nil) {
			t.Fatalf("open tool after interrupted completion = %+v", tool)
		}
	}
	thoughts, err := repo.ListSessionThinkingRecordsByAssistantMessageIDs(readCtx, req.SessionID, []string{assistant.ID})
	if err != nil || len(thoughts) != 2 || thoughts[1].Status != "completed" || thoughts[1].Content != "interrupted thought" || thoughts[1].FinishedAt == nil {
		t.Fatalf("thinking records = %+v, %v", thoughts, err)
	}
	snapshot, err := repo.GetContextSnapshot(readCtx, req.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	var bound int
	for _, entry := range snapshot.Entries {
		if entry.RequestID == req.RequestID {
			if entry.SourceMessageID != assistant.ID {
				t.Fatalf("unbound context entry = %+v", entry)
			}
			bound++
		}
	}
	if bound < 2 {
		t.Fatalf("expected assistant and tool context entries, got %d", bound)
	}
}

func TestRunTurnAcceptsBasicChatStore(t *testing.T) {
	repo := newTestRepo(t)
	t.Cleanup(func() { _ = repo.Close() })
	req := turnTestRequest(t, repo)
	store := struct{ domain.ChatStore }{repo}
	service := NewChatService(store, nil, llmsvc.NewFactory(&fakeThinkingProvider{}), nil, nil)
	if _, err := service.RunTurn(context.Background(), req, AgentCallbacks{}); err != nil {
		t.Fatal(err)
	}
	assistant := recordedAssistant(t, repo, req)
	thoughts, err := repo.ListSessionThinkingRecordsByAssistantMessageIDs(context.Background(), req.SessionID, []string{assistant.ID})
	if err != nil || len(thoughts) != 1 || thoughts[0].Content != "reasoning" {
		t.Fatalf("fallback thinking records = %+v, %v", thoughts, err)
	}
}

func recordedAssistant(t *testing.T, repo *repositories.Repository, req ChatTurnRequest) domain.Message {
	t.Helper()
	messages, err := repo.ListAllSessionMessages(context.Background(), req.SessionID, -1)
	if err != nil || len(messages) != 2 || messages[1].Role != "assistant" || messages[1].RequestID != req.RequestID {
		t.Fatalf("recorded turn = %+v, %v", messages, err)
	}
	return messages[1]
}
