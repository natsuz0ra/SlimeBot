package chat

import (
	"context"
	"strings"
	"testing"

	"slimebot/backend/internal/constants"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/repositories"
)

func TestResolveToolInvocation_ActivateSkill(t *testing.T) {
	tc := ToolCallInfo{
		ID:        "call_1",
		Name:      "activate_skill",
		Arguments: `{"name":"demo-skill"}`,
	}

	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if invocation.toolName != "activate_skill" {
		t.Fatalf("unexpected toolName: %s", invocation.toolName)
	}
	if invocation.command != "activate" {
		t.Fatalf("unexpected command: %s", invocation.command)
	}
	if invocation.requiresApproval {
		t.Fatal("activate_skill should not require approval")
	}
}

func TestResolveToolInvocation_SearchMemory(t *testing.T) {
	tc := ToolCallInfo{
		ID:        "call_2",
		Name:      constants.SearchMemoryTool,
		Arguments: `{"query":"golang"}`,
	}

	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if invocation.toolName != constants.SearchMemoryTool {
		t.Fatalf("unexpected toolName: %s", invocation.toolName)
	}
	if invocation.command != "query" {
		t.Fatalf("unexpected command: %s", invocation.command)
	}
	if invocation.requiresApproval {
		t.Fatal("search_memory should not require approval")
	}
}

func TestExecuteInvocation_SearchMemory_OncePerResponse(t *testing.T) {
	repo := newTestRepo(t)
	memorySvc := NewMemoryService(repo, nil)
	agent := &AgentService{memory: memorySvc}

	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          "other-session",
		Summary:            "用户偏好 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 12,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}

	memoryUsed := false
	first := agent.executeInvocation(
		context.Background(),
		ToolCallInfo{ID: "call_3", Name: constants.SearchMemoryTool},
		resolvedToolInvocation{toolName: constants.SearchMemoryTool, command: "query"},
		map[string]string{"query": "golang"},
		"current-session",
		nil,
		&memoryUsed,
	)
	if first == nil || strings.TrimSpace(first.Error) != "" {
		t.Fatalf("expected first call success, got err=%v", first)
	}
	if !strings.Contains(first.Output, "<memory_query_result>") {
		t.Fatalf("expected memory query output, got: %q", first.Output)
	}

	second := agent.executeInvocation(
		context.Background(),
		ToolCallInfo{ID: "call_4", Name: constants.SearchMemoryTool},
		resolvedToolInvocation{toolName: constants.SearchMemoryTool, command: "query"},
		map[string]string{"query": "golang"},
		"current-session",
		nil,
		&memoryUsed,
	)
	if second == nil || !strings.Contains(second.Error, "at most once") {
		t.Fatalf("expected once-per-response error, got: %+v", second)
	}
}
