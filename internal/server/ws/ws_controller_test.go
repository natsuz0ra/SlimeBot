package ws

import (
	"testing"
	"time"

	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"
)

func TestChatTimingPayloadsUseServerReceivedAndDoneTimes(t *testing.T) {
	sessionID := "session-1"
	receivedAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	doneSentAt := receivedAt.Add(2750 * time.Millisecond)

	startPayload := buildChatStartPayload(sessionID, "turn-1", receivedAt)
	donePayload := buildChatDonePayload(sessionID, nil, receivedAt, doneSentAt)

	if startPayload.StartedAt != receivedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected start payload: %+v", startPayload)
	}
	if donePayload.FinishedAt != doneSentAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected done finishedAt: %+v", donePayload)
	}
	if donePayload.DurationMs != int64(2750) {
		t.Fatalf("unexpected done durationMs: %+v", donePayload)
	}
}

func TestBuildMessageEditedPayload(t *testing.T) {
	editedAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	payload := buildMessageEditedPayload("session-1", "message-1", "edited text", editedAt)

	if payload.Type != "message_edited" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload.SessionID != "session-1" || payload.MessageID != "message-1" || payload.Content != "edited text" {
		t.Fatalf("unexpected edit payload: %+v", payload)
	}
	if payload.CreatedAt != editedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected edit createdAt: %+v", payload)
	}
}

func TestApprovalBrokerDeliversResponseResolvedBeforeRegister(t *testing.T) {
	broker := newApprovalBroker()
	broker.Resolve("call-fast", chatsvc.ApprovalResponse{
		ToolCallID: "call-fast",
		Approved:   true,
	})

	ch := broker.Register("call-fast")

	select {
	case resp := <-ch:
		if resp.ToolCallID != "call-fast" || !resp.Approved {
			t.Fatalf("unexpected approval response: %+v", resp)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected queued approval response")
	}

	broker.Remove("call-fast")
}

func TestBuildTodoUpdatePayloadIncludesSessionScopedItems(t *testing.T) {
	updatedAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	payload := buildTodoUpdatePayload("session-1", chatsvc.TodoUpdate{
		Note: "working",
		Items: []chatsvc.TodoItem{{
			ID:      "inspect",
			Content: "Inspect flow",
			Status:  "in_progress",
		}},
	}, updatedAt)

	if payload.Type != "todo_update" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %+v", payload)
	}
	if payload.Note != "working" {
		t.Fatalf("unexpected note: %+v", payload)
	}
	if payload.UpdatedAt != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected updatedAt: %+v", payload)
	}
	items := payload.Items
	if len(items) != 1 || items[0].Status != "in_progress" {
		t.Fatalf("unexpected items: %+v", payload.Items)
	}
}

func TestBuildSubagentStartPayloadIncludesTitleAndTask(t *testing.T) {
	payload := buildSubagentStartPayload("session-1", chatsvc.AgentEventMeta{
		ParentToolCallID: "parent-call",
		SubagentRunID:    "run-1",
		TeamRunID:        "team-1",
		MemberRunID:      "member-1",
	}, "Inspect UI cards", "Inspect UI cards and report exact files")

	if payload.Type != "subagent_start" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload.SessionID != "session-1" {
		t.Fatalf("unexpected session id: %+v", payload)
	}
	if payload.ParentToolCallID != "parent-call" {
		t.Fatalf("unexpected parent id: %+v", payload)
	}
	if payload.SubagentRunID != "run-1" {
		t.Fatalf("unexpected run id: %+v", payload)
	}
	if payload.TeamRunID != "team-1" || payload.MemberRunID != "member-1" {
		t.Fatalf("unexpected team identity: %+v", payload)
	}
	if payload.Title != "Inspect UI cards" {
		t.Fatalf("unexpected title: %+v", payload)
	}
	if payload.Task != "Inspect UI cards and report exact files" {
		t.Fatalf("unexpected task: %+v", payload)
	}
}

func TestBuildTeamPayloadsIncludeLifecycleFields(t *testing.T) {
	finishedAt := time.Now()
	run := domain.TeamRun{
		ID: "team-1", SessionID: "session-1", RequestID: "request-1", Status: domain.TeamRunStatusSucceeded,
		MaxMembers: 8, MaxParallel: 4, StartedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt,
	}
	start := buildTeamStartPayload(run)
	done := buildTeamDonePayload(run)
	if start.Type != "team_start" || start.TeamRunID != "team-1" || start.RequestID != "request-1" {
		t.Fatalf("team start = %+v", start)
	}
	if done.Type != "team_done" || done.Status != domain.TeamRunStatusSucceeded || done.FinishedAt == "" {
		t.Fatalf("team done = %+v", done)
	}
}

func TestBuildTeamMemberQueuedPayloadIncludesMemberFields(t *testing.T) {
	createdAt := time.Now()
	member := domain.TeamMemberRun{
		ID: "member-1", TeamRunID: "team-1", ToolCallID: "tool-1", Title: "Research",
		Task: "Inspect runtime", ModelConfigID: "model-1", Status: domain.TeamMemberRunStatusQueued, CreatedAt: createdAt,
	}
	payload := buildTeamMemberQueuedPayload("session-1", member)
	if payload.Type != "team_member_queued" || payload.SessionID != "session-1" {
		t.Fatalf("queued member payload = %+v", payload)
	}
	if payload.TeamRunID != "team-1" || payload.MemberRunID != "member-1" || payload.ToolCallID != "tool-1" {
		t.Fatalf("queued member identity = %+v", payload)
	}
	if payload.Status != domain.TeamMemberRunStatusQueued || payload.CreatedAt == "" {
		t.Fatalf("queued member lifecycle = %+v", payload)
	}
}

func TestBuildContextUsagePayloadIncludesPercentages(t *testing.T) {
	payload := buildContextUsagePayload("session-1", chatsvc.ContextUsage{
		SessionID:          "session-1",
		ModelConfigID:      "model-1",
		UsedTokens:         420_000,
		TotalTokens:        1_000_000,
		UsedPercent:        42,
		AvailablePercent:   58,
		IsCompacted:        true,
		CompactedAt:        "2026-05-03T01:02:03Z",
		OutputBudgetTokens: 20_000,
		OutputBudgetUsed:   16_500,
	})

	if payload.Type != "context_usage" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload.SessionID != "session-1" || payload.ModelConfigID != "model-1" {
		t.Fatalf("unexpected identity: %+v", payload)
	}
	if payload.UsedTokens != 420_000 || payload.TotalTokens != 1_000_000 {
		t.Fatalf("unexpected token counts: %+v", payload)
	}
	if payload.UsedPercent != 42 || payload.AvailablePercent != 58 {
		t.Fatalf("unexpected percentages: %+v", payload)
	}
	if payload.IsCompacted != true || payload.CompactedAt != "2026-05-03T01:02:03Z" {
		t.Fatalf("unexpected compact state: %+v", payload)
	}
	if payload.OutputBudgetTokens != int64(20_000) || payload.OutputBudgetUsed != int64(16_500) {
		t.Fatalf("unexpected output budget fields: %+v", payload)
	}
}

func TestBuildContextCompactedPayloadIncludesUsage(t *testing.T) {
	payload := buildContextCompactedPayload("session-1", chatsvc.ContextUsage{
		SessionID:        "session-1",
		ModelConfigID:    "model-1",
		UsedTokens:       120_000,
		TotalTokens:      500_000,
		UsedPercent:      24,
		AvailablePercent: 76,
		IsCompacted:      true,
	})

	if payload.Type != "context_compacted" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	usage := payload.Usage
	if usage.UsedPercent != 24 || !usage.IsCompacted {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestBuildPostDoneContextUsagePayloadsAlwaysIncludesFinalUsage(t *testing.T) {
	usage := chatsvc.ContextUsage{
		SessionID:        "session-1",
		ModelConfigID:    "model-1",
		UsedTokens:       430_000,
		TotalTokens:      1_000_000,
		UsedPercent:      43,
		AvailablePercent: 57,
	}

	payloads := buildPostDoneContextUsagePayloads("session-1", usage, false)

	if len(payloads) != 1 {
		t.Fatalf("expected only final usage payload, got %+v", payloads)
	}
	if usageEvent, ok := payloads[0].(wsContextUsage); !ok || usageEvent.UsedPercent != 43 {
		t.Fatalf("unexpected final usage payload: %+v", payloads[0])
	}
}

func TestBuildPostDoneContextUsagePayloadsIncludesCompactedEventWhenSummaryUpdated(t *testing.T) {
	usage := chatsvc.ContextUsage{
		SessionID:        "session-1",
		ModelConfigID:    "model-1",
		UsedTokens:       120_000,
		TotalTokens:      500_000,
		UsedPercent:      24,
		AvailablePercent: 76,
		IsCompacted:      true,
	}

	payloads := buildPostDoneContextUsagePayloads("session-1", usage, true)

	if len(payloads) != 2 {
		t.Fatalf("expected final usage and compacted payloads, got %+v", payloads)
	}
	if _, ok := payloads[0].(wsContextUsage); !ok {
		t.Fatalf("expected context_usage first, got %+v", payloads[0])
	}
	if _, ok := payloads[1].(wsContextCompacted); !ok {
		t.Fatalf("expected context_compacted second, got %+v", payloads[1])
	}
}
