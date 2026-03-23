package repositories

import (
	"context"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestUpsertToolCallStart_UpdatesExistingRow(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_tool_calls_upsert"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	first := domain.ToolCallStartRecordInput{
		SessionID:        session.ID,
		RequestID:        "r1",
		ToolCallID:       "tc1",
		ToolName:         "exec",
		Command:          "run",
		Params:           map[string]string{"cmd": "echo 1"},
		Status:           "pending",
		RequiresApproval: true,
		StartedAt:        time.Now().Add(-1 * time.Minute),
	}
	if err := repo.UpsertToolCallStart(context.Background(), first); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}

	second := first
	second.ToolName = "web_search"
	second.Command = "search"
	second.Status = "executing"
	second.RequiresApproval = false
	second.Params = map[string]string{"q": "golang"}
	second.StartedAt = time.Now()
	if err := repo.UpsertToolCallStart(context.Background(), second); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	rows, err := repo.ListSessionToolCallRecords(session.ID)
	if err != nil {
		t.Fatalf("list tool call records failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ToolName != "web_search" || rows[0].Command != "search" || rows[0].Status != "executing" {
		t.Fatalf("row was not updated: %+v", rows[0])
	}
}
