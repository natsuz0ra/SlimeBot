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
