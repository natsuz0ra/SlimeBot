package repositories

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCompleteChatTurnRollsBackAllWrites(t *testing.T) {
	ctx := context.Background()
	db := NewSQLiteDBTest(t, "complete_turn_rollback")
	repo := New(db)
	t.Cleanup(func() { _ = repo.Close() })
	input := domain.CompleteChatTurnInput{
		SessionID: "session", RequestID: "turn", ContextRequestID: "context",
		Interrupted: true, TokenUsage: &llmsvc.TokenUsage{OutputTokens: 12},
	}
	now := time.Now()
	if err := repo.UpsertToolCallStart(ctx, domain.ToolCallStartRecordInput{
		SessionID: input.SessionID, RequestID: input.RequestID, ToolCallID: "tool",
		Status: constants.ToolCallStatusExecuting, StartedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertThinkingStart(ctx, domain.ThinkingStartRecordInput{
		SessionID: input.SessionID, RequestID: input.RequestID, ThinkingID: "thinking", StartedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	entry := domain.ContextEntry{ScopeID: input.SessionID, RequestID: input.ContextRequestID, MessageSeq: 1, Fidelity: "exact", Payload: "{}"}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER block_context_bind BEFORE UPDATE OF source_message_id ON context_entries
		BEGIN SELECT RAISE(ABORT, 'injected context bind failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if result, err := repo.CompleteChatTurn(ctx, input); result != nil || err == nil || !strings.Contains(err.Error(), "injected context bind failure") {
		t.Fatalf("completion = %+v, %v", result, err)
	}

	var count int64
	if err := db.Model(&domain.Message{}).Where("session_id = ?", input.SessionID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("messages after rollback = %d, %v", count, err)
	}
	var tool domain.ToolCallRecord
	if err := db.Take(&tool).Error; err != nil || tool.AssistantMessageID != nil || tool.Status != constants.ToolCallStatusExecuting || tool.FinishedAt != nil {
		t.Fatalf("tool after rollback = %+v, %v", tool, err)
	}
	var thinking domain.ThinkingRecord
	if err := db.Take(&thinking).Error; err != nil || thinking.AssistantMessageID != nil || thinking.Status != thinkingStatusStreaming || thinking.FinishedAt != nil {
		t.Fatalf("thinking after rollback = %+v, %v", thinking, err)
	}
	snapshot, err := repo.GetContextSnapshot(ctx, input.SessionID)
	if err != nil || snapshot.Head.HistoryRevision != 0 || len(snapshot.Entries) != 1 || snapshot.Entries[0].SourceMessageID != "" {
		t.Fatalf("context after rollback = %+v, %v", snapshot, err)
	}

	if err := db.Exec("DROP TRIGGER block_context_bind").Error; err != nil {
		t.Fatal(err)
	}
	result, err := repo.CompleteChatTurn(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	message := result.AssistantMessage
	if result.Deduplicated || message.Seq != 1 || message.RequestID != input.RequestID || !message.IsInterrupted || !message.IsStopPlaceholder {
		t.Fatalf("assistant after retry = %+v", message)
	}
	if err := db.Take(&tool).Error; err != nil || tool.AssistantMessageID == nil || *tool.AssistantMessageID != message.ID || tool.Status != constants.ToolCallStatusError {
		t.Fatalf("tool after retry = %+v, %v", tool, err)
	}
	if err := db.Take(&thinking).Error; err != nil || thinking.AssistantMessageID == nil || *thinking.AssistantMessageID != message.ID || thinking.Status != thinkingStatusCompleted {
		t.Fatalf("thinking after retry = %+v, %v", thinking, err)
	}
	snapshot, err = repo.GetContextSnapshot(ctx, input.SessionID)
	if err != nil || snapshot.Head.HistoryRevision != 1 || len(snapshot.Entries) != 1 || snapshot.Entries[0].SourceMessageID != message.ID {
		t.Fatalf("context after retry = %+v, %v", snapshot, err)
	}
}

func TestCompleteChatTurnDeduplicatesAssistantBySessionAndRequest(t *testing.T) {
	ctx := context.Background()
	repo := New(NewSQLiteDBTest(t, "complete_turn_idempotency"))
	t.Cleanup(func() { _ = repo.Close() })
	input := domain.CompleteChatTurnInput{
		SessionID: "session", RequestID: "request", Content: "original answer",
		TokenUsage: &llmsvc.TokenUsage{InputTokens: 4, OutputTokens: 2},
	}
	if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
		SessionID: input.SessionID, Role: "user", RequestID: input.RequestID, Content: "question",
	}); err != nil {
		t.Fatal(err)
	}
	first, err := repo.CompleteChatTurn(ctx, input)
	if err != nil || first.Deduplicated {
		t.Fatalf("initial completion = %+v, %v", first, err)
	}
	retry := input
	retry.Content, retry.Interrupted, retry.TokenUsage = "changed", true, nil
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			replayed, err := repo.CompleteChatTurn(ctx, retry)
			if err != nil || !replayed.Deduplicated {
				t.Errorf("retried completion = %+v, %v", replayed, err)
				return
			}
			message := replayed.AssistantMessage
			if message.ID != first.AssistantMessage.ID || message.Content != input.Content || message.IsInterrupted ||
				message.TokenUsage == nil || *message.TokenUsage != *input.TokenUsage {
				t.Errorf("retried assistant = %+v", message)
			}
		})
	}
	wg.Wait()
	items, err := repo.ListAllSessionMessages(ctx, input.SessionID, -1)
	if err != nil || len(items) != 2 || items[1].ID != first.AssistantMessage.ID {
		t.Fatalf("history = %+v, %v", items, err)
	}
	if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{
		SessionID: input.SessionID, Role: "assistant", RequestID: input.RequestID, Content: "duplicate",
	}); err == nil {
		t.Fatal("database accepted a second assistant with the same session and request")
	}
	other := input
	other.SessionID = "other-session"
	second, err := repo.CompleteChatTurn(ctx, other)
	if err != nil || second.Deduplicated || second.AssistantMessage.ID == first.AssistantMessage.ID {
		t.Fatalf("completion in another session = %+v, %v", second, err)
	}
}

func TestCompleteChatTurnMigratesLegacyMessages(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE messages (
		id text PRIMARY KEY, session_id text NOT NULL, role text NOT NULL, content text NOT NULL,
		is_interrupted numeric NOT NULL DEFAULT false, is_stop_placeholder numeric NOT NULL DEFAULT false,
		attachments_json text NOT NULL DEFAULT '[]', token_usage_json text NOT NULL DEFAULT '',
		created_at datetime, seq integer NOT NULL DEFAULT 0
	)`).Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := db.Exec("INSERT INTO messages (id, session_id, role, content, seq) VALUES (?, 'legacy', 'assistant', 'old answer', ?)", fmt.Sprint(i), i).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := New(db).Close(); err != nil {
		t.Fatal(err)
	}
	db, err = NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	t.Cleanup(func() { _ = repo.Close() })
	var messages []domain.Message
	if err := db.Order("seq").Find(&messages).Error; err != nil || len(messages) != 2 || messages[0].RequestID != "" || messages[1].RequestID != "" {
		t.Fatalf("legacy messages after migration = %+v, %v", messages, err)
	}
	for seq := int64(3); seq <= 4; seq++ {
		result, err := repo.CompleteChatTurn(ctx, domain.CompleteChatTurnInput{SessionID: "legacy", Content: "new answer"})
		if err != nil || result.Deduplicated || result.AssistantMessage.Seq != seq {
			t.Fatalf("completion with empty request = %+v, %v", result, err)
		}
	}
}
