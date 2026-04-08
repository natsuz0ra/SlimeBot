package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	llmsvc "slimebot/internal/services/llm"
)

func TestBuildContextMessages_SystemPrefixStableAndNoLocalDateTime(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	ctx := context.Background()

	msgs1, err := svc.BuildContextMessages(ctx, "session-1", llmsvc.ModelRuntimeConfig{})
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

	msgs2, err := svc.BuildContextMessages(ctx, "session-1", llmsvc.ModelRuntimeConfig{})
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

func TestBuildContextMessages_IncludesConfigDirInCLI(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	svc.SetRunContext(RunContext{
		ConfigHomeDir:        "/home/user/.slimebot",
		ConfigDirDescription: "/home/user/.slimebot/\n  skills/\n  storage/\n",
		WorkingDir:           "/home/user/project",
		IsCLI:                true,
	})
	ctx := context.Background()

	msgs, err := svc.BuildContextMessages(ctx, "session-cli", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 找到 runtime environment 消息
	found := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "Config directory") {
			found = true
			if !strings.Contains(m.Content, "/home/user/.slimebot") {
				t.Fatal("expected config home dir in runtime prompt")
			}
			if !strings.Contains(m.Content, "skills/") {
				t.Fatal("expected skills/ in directory listing")
			}
			if !strings.Contains(m.Content, "Current working directory: /home/user/project") {
				t.Fatal("expected working dir in CLI mode runtime prompt")
			}
		}
	}
	if !found {
		t.Fatal("expected runtime environment message with config directory")
	}
}

func TestBuildContextMessages_ServerMode_NoWorkingDir(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	svc.SetRunContext(RunContext{
		ConfigHomeDir: "/home/user/.slimebot",
		IsCLI:         false,
	})
	ctx := context.Background()

	msgs, err := svc.BuildContextMessages(ctx, "session-srv", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "Config directory") {
			if strings.Contains(m.Content, "Current working directory") {
				t.Fatal("server mode should not include working directory")
			}
		}
	}
}

func TestBuildContextMessages_NoRunContext_OmitsConfigDir(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewChatService(repo, nil, nil, nil, nil)
	// 不设置 RunContext（零值）
	ctx := context.Background()

	msgs, err := svc.BuildContextMessages(ctx, "session-norc", llmsvc.ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "## Runtime Environment") {
			if strings.Contains(m.Content, "Config directory") {
				t.Fatal("expected no config directory when RunContext is zero-valued")
			}
			if strings.Contains(m.Content, "Current working directory") {
				t.Fatal("expected no working directory when RunContext is zero-valued")
			}
		}
	}
}
