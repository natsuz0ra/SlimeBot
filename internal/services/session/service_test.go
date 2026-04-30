package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"slimebot/internal/domain"
)

type storeStub struct {
	seenCtx         context.Context
	sessions        []domain.Session
	messages        []domain.Message
	toolRecords     []domain.ToolCallRecord
	thinkingRecords []domain.ThinkingRecord
}

func (s *storeStub) ListSessions(ctx context.Context, limit int, offset int, query string) ([]domain.Session, error) {
	s.seenCtx = ctx
	return s.sessions, nil
}

func (s *storeStub) CreateSession(ctx context.Context, name string) (*domain.Session, error) {
	s.seenCtx = ctx
	return &domain.Session{ID: "created", Name: name}, nil
}

func (s *storeStub) RenameSessionByUser(ctx context.Context, id, name string) error {
	s.seenCtx = ctx
	return nil
}

func (s *storeStub) DeleteSession(ctx context.Context, id string) error {
	s.seenCtx = ctx
	return nil
}

func (s *storeStub) ListSessionMessagesPage(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error) {
	s.seenCtx = ctx
	return s.messages, false, nil
}

func (s *storeStub) ListSessionToolCallRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error) {
	s.seenCtx = ctx
	return s.toolRecords, nil
}

func (s *storeStub) ListSessionThinkingRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.ThinkingRecord, error) {
	s.seenCtx = ctx
	return s.thinkingRecords, nil
}

func TestGetMessageHistoryBuildsToolThinkingAndReplyTiming(t *testing.T) {
	sessionID := "session-1"
	assistantID := "assistant-1"
	assistantIDPtr := assistantID
	userAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	assistantAt := userAt.Add(2500 * time.Millisecond)
	store := &storeStub{
		messages: []domain.Message{
			{ID: "user-1", SessionID: sessionID, Role: "user", Content: "hello", CreatedAt: userAt, Seq: 1},
			{ID: assistantID, SessionID: sessionID, Role: "assistant", Content: "hi", IsInterrupted: true, CreatedAt: assistantAt, Seq: 2},
		},
		toolRecords: []domain.ToolCallRecord{{
			ToolCallID:         "tool-1",
			ToolName:           "exec",
			Command:            "run",
			ParamsJSON:         `{"cmd":"pwd"}`,
			Status:             "executing",
			AssistantMessageID: &assistantIDPtr,
			StartedAt:          assistantAt,
		}},
		thinkingRecords: []domain.ThinkingRecord{{
			ThinkingID:         "think-1",
			Content:            "reasoning",
			Status:             "streaming",
			AssistantMessageID: &assistantIDPtr,
			StartedAt:          assistantAt,
		}},
	}

	got, err := NewSessionService(store).GetMessageHistory(context.Background(), sessionID, 10, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("GetMessageHistory failed: %v", err)
	}

	if got.ReplyTimingByAssistantMessageID[assistantID].DurationMs != 2500 {
		t.Fatalf("unexpected reply timing: %+v", got.ReplyTimingByAssistantMessageID)
	}
	tool := got.ToolCallsByAssistantMessageID[assistantID][0]
	if tool.Status != "error" || tool.Error != "Execution cancelled." || tool.Params["cmd"] != "pwd" {
		t.Fatalf("unexpected tool history: %+v", tool)
	}
	thinking := got.ThinkingByAssistantMessageID[assistantID][0]
	if thinking.Status != "completed" || thinking.Content != "reasoning" {
		t.Fatalf("unexpected thinking history: %+v", thinking)
	}
}

func TestSessionServicePassesContextToStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &storeStub{}

	_, err := NewSessionService(store).Create(ctx, "demo")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if !errors.Is(store.seenCtx.Err(), context.Canceled) {
		t.Fatalf("expected canceled context to reach store, got %v", store.seenCtx.Err())
	}
}
