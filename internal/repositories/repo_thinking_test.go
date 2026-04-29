package repositories

import (
	"context"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestThinkingRecordLifecycle_BindsAndListsByAssistantMessage(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_thinking_lifecycle"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	startedAt := time.Now().Add(-2 * time.Second).UTC()
	if err := repo.UpsertThinkingStart(context.Background(), domain.ThinkingStartRecordInput{
		SessionID:  session.ID,
		RequestID:  "request-1",
		ThinkingID: "think-1",
		StartedAt:  startedAt,
	}); err != nil {
		t.Fatalf("start thinking failed: %v", err)
	}
	if err := repo.AppendThinkingChunk(context.Background(), domain.ThinkingChunkRecordInput{
		SessionID:  session.ID,
		RequestID:  "request-1",
		ThinkingID: "think-1",
		Chunk:      "first ",
	}); err != nil {
		t.Fatalf("append first chunk failed: %v", err)
	}
	if err := repo.AppendThinkingChunk(context.Background(), domain.ThinkingChunkRecordInput{
		SessionID:  session.ID,
		RequestID:  "request-1",
		ThinkingID: "think-1",
		Chunk:      "second",
	}); err != nil {
		t.Fatalf("append second chunk failed: %v", err)
	}

	finishedAt := startedAt.Add(1500 * time.Millisecond)
	if err := repo.FinishThinking(context.Background(), domain.ThinkingFinishRecordInput{
		SessionID:  session.ID,
		RequestID:  "request-1",
		ThinkingID: "think-1",
		FinishedAt: finishedAt,
	}); err != nil {
		t.Fatalf("finish thinking failed: %v", err)
	}

	assistant, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "<!-- THINKING:think-1 -->\nanswer",
	})
	if err != nil {
		t.Fatalf("add assistant message failed: %v", err)
	}
	if err := repo.BindThinkingRecordsToAssistantMessage(context.Background(), session.ID, "request-1", assistant.ID); err != nil {
		t.Fatalf("bind thinking failed: %v", err)
	}

	records, err := repo.ListSessionThinkingRecordsByAssistantMessageIDs(session.ID, []string{assistant.ID})
	if err != nil {
		t.Fatalf("list thinking records failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 thinking record, got %d", len(records))
	}
	if records[0].ThinkingID != "think-1" || records[0].Content != "first second" {
		t.Fatalf("unexpected thinking record: %+v", records[0])
	}
	if records[0].Status != "completed" {
		t.Fatalf("expected completed status, got %q", records[0].Status)
	}
	if records[0].DurationMs != 1500 {
		t.Fatalf("expected duration 1500ms, got %d", records[0].DurationMs)
	}
	if records[0].AssistantMessageID == nil || *records[0].AssistantMessageID != assistant.ID {
		t.Fatalf("record was not bound to assistant message: %+v", records[0])
	}
}

func TestThinkingRecordLifecycle_PreservesSubagentOwnership(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "repo_thinking_subagent_owner"))
	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	if err := repo.UpsertThinkingStart(context.Background(), domain.ThinkingStartRecordInput{
		SessionID:        session.ID,
		RequestID:        "request-1",
		ThinkingID:       "think-child",
		ParentToolCallID: "parent-tool",
		SubagentRunID:    "sub-run",
	}); err != nil {
		t.Fatalf("start thinking failed: %v", err)
	}
	if err := repo.AppendThinkingChunk(context.Background(), domain.ThinkingChunkRecordInput{
		SessionID:  session.ID,
		RequestID:  "request-1",
		ThinkingID: "think-child",
		Chunk:      "child reasoning",
	}); err != nil {
		t.Fatalf("append chunk failed: %v", err)
	}

	assistant, err := repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: session.ID,
		Role:      "assistant",
		Content:   "<!-- TOOL_CALL:parent-tool -->\nanswer",
	})
	if err != nil {
		t.Fatalf("add assistant message failed: %v", err)
	}
	if err := repo.BindThinkingRecordsToAssistantMessage(context.Background(), session.ID, "request-1", assistant.ID); err != nil {
		t.Fatalf("bind thinking failed: %v", err)
	}

	records, err := repo.ListSessionThinkingRecordsByAssistantMessageIDs(session.ID, []string{assistant.ID})
	if err != nil {
		t.Fatalf("list thinking records failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 thinking record, got %d", len(records))
	}
	if records[0].ParentToolCallID != "parent-tool" || records[0].SubagentRunID != "sub-run" {
		t.Fatalf("subagent ownership was not preserved: %+v", records[0])
	}
}
