package ws

import (
	"testing"
	"time"

	chatsvc "slimebot/internal/services/chat"
)

func TestChatTimingPayloadsUseServerReceivedAndDoneTimes(t *testing.T) {
	sessionID := "session-1"
	receivedAt := time.Date(2026, 4, 29, 1, 2, 3, 0, time.UTC)
	doneSentAt := receivedAt.Add(2750 * time.Millisecond)

	startPayload := buildChatStartPayload(sessionID, receivedAt)
	donePayload := buildChatDonePayload(sessionID, nil, receivedAt, doneSentAt)

	if startPayload["startedAt"] != receivedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected start payload: %+v", startPayload)
	}
	if donePayload["finishedAt"] != doneSentAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected done finishedAt: %+v", donePayload)
	}
	if donePayload["durationMs"] != int64(2750) {
		t.Fatalf("unexpected done durationMs: %+v", donePayload)
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

	if payload["type"] != "todo_update" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload["sessionId"] != "session-1" {
		t.Fatalf("unexpected session id: %+v", payload)
	}
	if payload["note"] != "working" {
		t.Fatalf("unexpected note: %+v", payload)
	}
	if payload["updatedAt"] != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected updatedAt: %+v", payload)
	}
	items, ok := payload["items"].([]chatsvc.TodoItem)
	if !ok || len(items) != 1 || items[0].Status != "in_progress" {
		t.Fatalf("unexpected items: %+v", payload["items"])
	}
}

func TestBuildSubagentStartPayloadIncludesTitleAndTask(t *testing.T) {
	payload := buildSubagentStartPayload("session-1", "parent-call", "run-1", "Inspect UI cards", "Inspect UI cards and report exact files")

	if payload["type"] != "subagent_start" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload["sessionId"] != "session-1" {
		t.Fatalf("unexpected session id: %+v", payload)
	}
	if payload["parentToolCallId"] != "parent-call" {
		t.Fatalf("unexpected parent id: %+v", payload)
	}
	if payload["subagentRunId"] != "run-1" {
		t.Fatalf("unexpected run id: %+v", payload)
	}
	if payload["title"] != "Inspect UI cards" {
		t.Fatalf("unexpected title: %+v", payload)
	}
	if payload["task"] != "Inspect UI cards and report exact files" {
		t.Fatalf("unexpected task: %+v", payload)
	}
}

func TestBuildContextUsagePayloadIncludesPercentages(t *testing.T) {
	payload := buildContextUsagePayload("session-1", chatsvc.ContextUsage{
		SessionID:        "session-1",
		ModelConfigID:    "model-1",
		UsedTokens:       420_000,
		TotalTokens:      1_000_000,
		UsedPercent:      42,
		AvailablePercent: 58,
		IsCompacted:      true,
		CompactedAt:      "2026-05-03T01:02:03Z",
	})

	if payload["type"] != "context_usage" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	if payload["sessionId"] != "session-1" || payload["modelConfigId"] != "model-1" {
		t.Fatalf("unexpected identity: %+v", payload)
	}
	if payload["usedTokens"] != 420_000 || payload["totalTokens"] != 1_000_000 {
		t.Fatalf("unexpected token counts: %+v", payload)
	}
	if payload["usedPercent"] != 42 || payload["availablePercent"] != 58 {
		t.Fatalf("unexpected percentages: %+v", payload)
	}
	if payload["isCompacted"] != true || payload["compactedAt"] != "2026-05-03T01:02:03Z" {
		t.Fatalf("unexpected compact state: %+v", payload)
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

	if payload["type"] != "context_compacted" {
		t.Fatalf("unexpected type: %+v", payload)
	}
	usage, ok := payload["usage"].(chatsvc.ContextUsage)
	if !ok {
		t.Fatalf("expected typed usage payload, got %+v", payload["usage"])
	}
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
	if payloads[0]["type"] != "context_usage" || payloads[0]["usedPercent"] != 43 {
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
	if payloads[0]["type"] != "context_usage" {
		t.Fatalf("expected context_usage first, got %+v", payloads[0])
	}
	if payloads[1]["type"] != "context_compacted" {
		t.Fatalf("expected context_compacted second, got %+v", payloads[1])
	}
}
