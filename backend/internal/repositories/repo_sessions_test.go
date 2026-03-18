package repositories

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/testutil"
)

func TestDeleteSession_DeletesSessionAndRelatedRecords(t *testing.T) {
	repo := New(testutil.NewSQLiteDB(t, "repo_sessions_delete_ok"))
	sessionID := uuid.NewString()

	if _, err := repo.CreateSessionWithID(sessionID, "to-delete"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := repo.db.Create(&models.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if err := repo.db.Create(&models.ToolCallRecord{
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

	if err := repo.DeleteSession(sessionID); err != nil {
		t.Fatalf("delete session failed: %v", err)
	}

	var sessionCount int64
	if err := repo.db.Model(&models.Session{}).Where("id = ?", sessionID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count session failed: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("expected session deleted, count=%d", sessionCount)
	}

	var messageCount int64
	if err := repo.db.Model(&models.Message{}).Where("session_id = ?", sessionID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count message failed: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("expected messages deleted, count=%d", messageCount)
	}

	var toolCallCount int64
	if err := repo.db.Model(&models.ToolCallRecord{}).Where("session_id = ?", sessionID).Count(&toolCallCount).Error; err != nil {
		t.Fatalf("count tool calls failed: %v", err)
	}
	if toolCallCount != 0 {
		t.Fatalf("expected tool calls deleted, count=%d", toolCallCount)
	}
}

func TestDeleteSession_RollsBackWhenToolCallDeleteFails(t *testing.T) {
	repo := New(testutil.NewSQLiteDB(t, "repo_sessions_delete_rollback"))
	sessionID := uuid.NewString()

	if _, err := repo.CreateSessionWithID(sessionID, "rollback-case"); err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := repo.db.Create(&models.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      "user",
		Content:   "persist me",
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if err := repo.db.Create(&models.ToolCallRecord{
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

	if err := repo.DeleteSession(sessionID); err == nil {
		t.Fatal("expected delete session to fail when tool call delete is blocked")
	}

	var sessionCount int64
	if err := repo.db.Model(&models.Session{}).Where("id = ?", sessionID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count session failed: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("expected session rollback to keep row, count=%d", sessionCount)
	}

	var messageCount int64
	if err := repo.db.Model(&models.Message{}).Where("session_id = ?", sessionID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count message failed: %v", err)
	}
	if messageCount != 1 {
		t.Fatalf("expected message rollback to keep row, count=%d", messageCount)
	}
}
