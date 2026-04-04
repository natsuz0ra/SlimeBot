package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	oaisvc "slimebot/internal/services/openai"
)

func TestBuildContextMessages_SystemPrefixStableAndNoLocalDateTime(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	ctx := context.Background()

	msgs1, err := svc.BuildContextMessages(ctx, "session-1", oaisvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("BuildContextMessages first call failed: %v", err)
	}
	if len(msgs1) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs1))
	}
	firstSystem1 := msgs1[0]
	if firstSystem1.Role != "system" {
		t.Fatalf("expected first message role system, got %q", firstSystem1.Role)
	}
	if strings.Contains(firstSystem1.Content, "## Runtime Environment") {
		t.Fatalf("first system prompt should not include runtime environment: %q", firstSystem1.Content)
	}
	if strings.Contains(firstSystem1.Content, "Local date:") || strings.Contains(firstSystem1.Content, "Local time:") {
		t.Fatalf("first system prompt should not include local date/time: %q", firstSystem1.Content)
	}

	time.Sleep(1200 * time.Millisecond)

	msgs2, err := svc.BuildContextMessages(ctx, "session-1", oaisvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("BuildContextMessages second call failed: %v", err)
	}
	if len(msgs2) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs2))
	}
	firstSystem2 := msgs2[0]
	if firstSystem1.Content != firstSystem2.Content {
		t.Fatalf("expected stable first system prompt across calls")
	}

	runtimeSystem := msgs1[1]
	if runtimeSystem.Role != "system" {
		t.Fatalf("expected runtime message role system, got %q", runtimeSystem.Role)
	}
	if !strings.Contains(runtimeSystem.Content, "## Runtime Environment") {
		t.Fatalf("expected runtime environment message, got %q", runtimeSystem.Content)
	}
	if strings.Contains(runtimeSystem.Content, "Local date:") || strings.Contains(runtimeSystem.Content, "Local time:") {
		t.Fatalf("runtime message should not include local date/time: %q", runtimeSystem.Content)
	}
}
