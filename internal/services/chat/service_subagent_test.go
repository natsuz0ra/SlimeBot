package chat

import (
	"context"
	"strings"
	"testing"
)

func TestBuildSubagentMessages_IncludesLanguageAndToolDiscipline(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil, nil)

	msgs, err := svc.BuildSubagentMessages(context.Background(), "session-1", "用中文检查提示词", "父级上下文")
	if err != nil {
		t.Fatalf("BuildSubagentMessages failed: %v", err)
	}

	var constraint string
	for _, msg := range msgs {
		if msg.Role == "system" && strings.Contains(msg.Content, "## Sub-agent mode") {
			constraint = msg.Content
			break
		}
	}
	if constraint == "" {
		t.Fatal("expected subagent constraint system block")
	}

	for _, want := range []string{
		"visible thinking/reasoning",
		"same language",
		"batch related inspection",
		"avoid repeated queries",
		"stop using tools",
		"confirmed facts, remaining gaps, and recommended next step",
	} {
		if !strings.Contains(constraint, want) {
			t.Fatalf("subagent constraint missing %q: %q", want, constraint)
		}
	}
}

func TestBuildSubagentMessages_PreservesTaskAndParentContextSections(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil, nil)

	msgs, err := svc.BuildSubagentMessages(context.Background(), "session-1", "Inspect prompt language rules", "Parent state summary")
	if err != nil {
		t.Fatalf("BuildSubagentMessages failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected messages")
	}

	userMsg := msgs[len(msgs)-1]
	if userMsg.Role != "user" {
		t.Fatalf("expected final message role user, got %q", userMsg.Role)
	}
	for _, want := range []string{
		"## Task\nInspect prompt language rules",
		"## Context from main assistant\nParent state summary",
	} {
		if !strings.Contains(userMsg.Content, want) {
			t.Fatalf("subagent user message missing %q: %q", want, userMsg.Content)
		}
	}
}
