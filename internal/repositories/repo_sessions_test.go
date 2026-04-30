package repositories

import (
	"context"
	"slimebot/internal/domain"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUpdateSessionTitle_UpdatesOnlyWhenUnlockedAndChanged(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_sessions_update_title"))
	ctx := context.Background()

	session, err := repo.CreateSession(ctx, "New Chat")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	updated, err := repo.UpdateSessionTitle(ctx, session.ID, "新的标题")
	if err != nil {
		t.Fatalf("update title failed: %v", err)
	}
	if !updated {
		t.Fatal("expected title update to report true")
	}

	reloaded, err := repo.GetSessionByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("reload session failed: %v", err)
	}
	if reloaded == nil || reloaded.Name != "新的标题" {
		t.Fatalf("unexpected session title after update: %+v", reloaded)
	}

	updated, err = repo.UpdateSessionTitle(ctx, session.ID, "新的标题")
	if err != nil {
		t.Fatalf("update title with same value failed: %v", err)
	}
	if updated {
		t.Fatal("expected same title update to report false")
	}

	if err := repo.RenameSessionByUser(context.Background(), session.ID, "用户标题"); err != nil {
		t.Fatalf("rename session by user failed: %v", err)
	}
	updated, err = repo.UpdateSessionTitle(ctx, session.ID, "自动标题")
	if err != nil {
		t.Fatalf("update title on locked session failed: %v", err)
	}
	if updated {
		t.Fatal("expected locked session update to report false")
	}

	reloaded, err = repo.GetSessionByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("reload locked session failed: %v", err)
	}
	if reloaded == nil || reloaded.Name != "用户标题" || !reloaded.IsTitleLocked {
		t.Fatalf("unexpected locked session state: %+v", reloaded)
	}
}

func TestDeleteSession_DeletesSessionAndRelatedRecords(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_sessions_delete_ok"))
	sessionID := uuid.NewString()

	if _, err := repo.CreateSessionWithID(context.Background(), sessionID, "to-delete"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := repo.db.Create(&domain.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if err := repo.db.Create(&domain.ToolCallRecord{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		RequestID:        uuid.NewString(),
		ToolCallID:       "call-1",
		ToolName:         "shell",
		Command:          "echo",
		ParamsJSON:       "{}",
		Status:           "done",
		RequiresApproval: false,
		StartedAt:        time.Now(),
	}).Error; err != nil {
		t.Fatalf("create tool call failed: %v", err)
	}

	if err := repo.DeleteSession(context.Background(), sessionID); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	var sessionCount int64
	if err := repo.db.Model(&domain.Session{}).Where("id = ?", sessionID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count session failed: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected session deleted, count=%d", sessionCount)
	}

	var messageCount int64
	if err := repo.db.Model(&domain.Message{}).Where("session_id = ?", sessionID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count message failed: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("expected messages deleted, count=%d", messageCount)
	}

	var toolCallCount int64
	if err := repo.db.Model(&domain.ToolCallRecord{}).Where("session_id = ?", sessionID).Count(&toolCallCount).Error; err != nil {
		t.Fatalf("count tool calls failed: %v", err)
	}
	if toolCallCount != 0 {
		t.Fatalf("expected tool calls deleted, count=%d", toolCallCount)
	}
}

func TestDeleteSession_RollsBackWhenToolCallDeleteFails(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_sessions_delete_rollback"))
	sessionID := uuid.NewString()

	if _, err := repo.CreateSessionWithID(context.Background(), sessionID, "rollback-case"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := repo.db.Create(&domain.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      "user",
		Content:   "persist me",
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if err := repo.db.Create(&domain.ToolCallRecord{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		RequestID:        uuid.NewString(),
		ToolCallID:       "call-rollback",
		ToolName:         "shell",
		Command:          "echo",
		ParamsJSON:       "{}",
		Status:           "done",
		RequiresApproval: false,
		StartedAt:        time.Now(),
	}).Error; err != nil {
		t.Fatalf("create tool call failed: %v", err)
	}

	if err := repo.db.Exec(`
CREATE TRIGGER block_tool_call_delete
BEFORE DELETE ON tool_call_records
BEGIN
  SELECT RAISE(ABORT, 'blocked for rollback test');
END;
`).Error; err != nil {
		t.Fatalf("create trigger failed: %v", err)
	}

	if err := repo.DeleteSession(context.Background(), sessionID); err == nil {
		t.Fatal("expected delete session to fail when tool call delete is blocked")
	}

	var sessionCount int64
	if err := repo.db.Model(&domain.Session{}).Where("id = ?", sessionID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count session failed: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("expected session rollback to keep row, count=%d", sessionCount)
	}

	var messageCount int64
	if err := repo.db.Model(&domain.Message{}).Where("session_id = ?", sessionID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count message failed: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("expected message rollback to keep row, count=%d", messageCount)
	}
}
