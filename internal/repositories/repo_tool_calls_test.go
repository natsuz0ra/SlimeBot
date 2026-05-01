package repositories

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"slimebot/internal/constants"
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
		Params:           map[string]any{"cmd": "echo 1"},
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
	second.Params = map[string]any{"q": "golang"}
	second.StartedAt = time.Now()
	if err := repo.UpsertToolCallStart(context.Background(), second); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}

	var rows []domain.ToolCallRecord
	if err := repo.db.Where("session_id = ?", session.ID).Order("started_at asc").Find(&rows).Error; err != nil {
		t.Fatalf("list tool call records failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ToolName != "web_search" || rows[0].Command != "search" || rows[0].Status != "executing" {
		t.Fatalf("row was not updated: %+v", rows[0])
	}
}

func TestUpsertToolCallStart_PersistsParentAndSubagentRun(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_tool_calls_parent"))
	session, err := repo.CreateSession(context.Background(), "s2")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	parentID := "parent-tc"
	in := domain.ToolCallStartRecordInput{
		SessionID:        session.ID,
		RequestID:        "r2",
		ToolCallID:       "child-tc",
		ToolName:         "web_search",
		Command:          "search",
		Params:           map[string]any{"query": "x"},
		Status:           "executing",
		RequiresApproval: false,
		StartedAt:        time.Now(),
		ParentToolCallID: parentID,
		SubagentRunID:    "run-uuid-1",
	}
	if err := repo.UpsertToolCallStart(context.Background(), in); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	var row domain.ToolCallRecord
	if err := repo.db.Where("session_id = ? AND tool_call_id = ?", session.ID, "child-tc").First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.ParentToolCallID != parentID {
		t.Fatalf("parent_tool_call_id: got %q want %q", row.ParentToolCallID, parentID)
	}
	if row.SubagentRunID != "run-uuid-1" {
		t.Fatalf("subagent_run_id: got %q", row.SubagentRunID)
	}

	in2 := in
	in2.ToolName = "exec"
	in2.ParentToolCallID = parentID + "-updated"
	in2.SubagentRunID = "run-uuid-2"
	if err := repo.UpsertToolCallStart(context.Background(), in2); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if err := repo.db.Where("session_id = ? AND tool_call_id = ?", session.ID, "child-tc").First(&row).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if row.ParentToolCallID != parentID+"-updated" || row.SubagentRunID != "run-uuid-2" {
		t.Fatalf("conflict update lost parent/subagent: %+v", row)
	}
}

func TestUpdateToolCallResult_PersistsMetadata(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_tool_calls_metadata"))
	session, err := repo.CreateSession(context.Background(), "s-meta")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if err := repo.UpsertToolCallStart(context.Background(), domain.ToolCallStartRecordInput{
		SessionID:        session.ID,
		RequestID:        "r-meta",
		ToolCallID:       "tc-meta",
		ToolName:         "file_edit",
		Command:          "edit",
		Params:           map[string]any{"file_path": "a.txt"},
		Status:           constants.ToolCallStatusExecuting,
		RequiresApproval: true,
		StartedAt:        time.Now(),
	}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	metadata := map[string]any{
		"filePath":  "a.txt",
		"operation": "Update",
		"diffLines": []map[string]any{{
			"kind":    "added",
			"newLine": 1,
			"text":    "ok",
		}},
	}
	if err := repo.UpdateToolCallResult(context.Background(), domain.ToolCallResultRecordInput{
		SessionID:  session.ID,
		RequestID:  "r-meta",
		ToolCallID: "tc-meta",
		Status:     constants.ToolCallStatusCompleted,
		Output:     "ok",
		Metadata:   metadata,
		FinishedAt: time.Now(),
	}); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var row domain.ToolCallRecord
	if err := repo.db.Where("session_id = ? AND tool_call_id = ?", session.ID, "tc-meta").First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(row.MetadataJSON), &got); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if got["filePath"] != "a.txt" || got["operation"] != "Update" {
		t.Fatalf("unexpected metadata: %s", row.MetadataJSON)
	}
}

func TestFinishOpenToolCallsForRequest_MarksOnlyPendingAndExecutingAsError(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_tool_calls_finish_open"))
	session, err := repo.CreateSession(context.Background(), "s3")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	for _, item := range []struct {
		id     string
		status string
	}{
		{"pending-tool", constants.ToolCallStatusPending},
		{"executing-tool", constants.ToolCallStatusExecuting},
		{"completed-tool", constants.ToolCallStatusCompleted},
	} {
		if err := repo.UpsertToolCallStart(context.Background(), domain.ToolCallStartRecordInput{
			SessionID:        session.ID,
			RequestID:        "request-finish",
			ToolCallID:       item.id,
			ToolName:         "run_subagent",
			Command:          "delegate",
			Params:           map[string]any{},
			Status:           item.status,
			RequiresApproval: false,
			StartedAt:        time.Now().Add(-time.Second),
		}); err != nil {
			t.Fatalf("upsert %s failed: %v", item.id, err)
		}
		if item.status == constants.ToolCallStatusCompleted {
			if err := repo.UpdateToolCallResult(context.Background(), domain.ToolCallResultRecordInput{
				SessionID:  session.ID,
				RequestID:  "request-finish",
				ToolCallID: item.id,
				Status:     constants.ToolCallStatusCompleted,
				Output:     "ok",
				FinishedAt: time.Now(),
			}); err != nil {
				t.Fatalf("complete tool failed: %v", err)
			}
		}
	}

	if err := repo.FinishOpenToolCallsForRequest(context.Background(), session.ID, "request-finish", "Execution cancelled."); err != nil {
		t.Fatalf("finish open tool calls failed: %v", err)
	}

	var rows []domain.ToolCallRecord
	if err := repo.db.Where("session_id = ?", session.ID).Order("tool_call_id asc").Find(&rows).Error; err != nil {
		t.Fatalf("list tool call records failed: %v", err)
	}
	got := map[string]domain.ToolCallRecord{}
	for _, row := range rows {
		got[row.ToolCallID] = row
	}
	for _, id := range []string{"pending-tool", "executing-tool"} {
		if got[id].Status != constants.ToolCallStatusError || got[id].Error != "Execution cancelled." || got[id].FinishedAt == nil {
			t.Fatalf("open tool was not marked error: %+v", got[id])
		}
	}
	if got["completed-tool"].Status != constants.ToolCallStatusCompleted || got["completed-tool"].Error != "" {
		t.Fatalf("completed tool should not be changed: %+v", got["completed-tool"])
	}
}
