package ws

import (
	"testing"
	"time"
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
