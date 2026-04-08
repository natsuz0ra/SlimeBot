package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/mcp"
	llmsvc "slimebot/internal/services/llm"
	memsvc "slimebot/internal/services/memory"
)

func TestResolveToolInvocation_ActivateSkill(t *testing.T) {
	tc := llmsvc.ToolCallInfo{
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
	tc := llmsvc.ToolCallInfo{
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
	memorySvc := memsvc.NewMemoryService(repo, nil)
	agent := &AgentService{memory: memorySvc}

	if _, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "current-session",
		TopicKey:       "golang",
		Title:          "Golang",
		Summary:        "用户在聊 golang",
		Keywords:       []string{"golang"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   4,
		TurnCount:      2,
		LastActiveAt:   time.Now(),
	}); err != nil {
		t.Fatalf("create episode failed: %v", err)
	}

	memoryUsed := false
	first := agent.executeInvocation(
		context.Background(),
		llmsvc.ToolCallInfo{ID: "call_3", Name: constants.SearchMemoryTool},
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
		llmsvc.ToolCallInfo{ID: "call_4", Name: constants.SearchMemoryTool},
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

func TestExecuteInvocation_SearchMemory_SearchesAcrossSessions(t *testing.T) {
	repo := newTestRepo(t)
	memorySvc := memsvc.NewMemoryService(repo, nil)
	agent := &AgentService{memory: memorySvc}

	if _, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "other-session",
		TopicKey:       "golang",
		Title:          "Cross Session Golang",
		Summary:        "来自其他会话的 Go 记忆",
		Keywords:       []string{"golang"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		TurnCount:      1,
		LastActiveAt:   time.Now(),
	}); err != nil {
		t.Fatalf("create episode failed: %v", err)
	}

	memoryUsed := false
	result := agent.executeInvocation(
		context.Background(),
		llmsvc.ToolCallInfo{ID: "call_5", Name: constants.SearchMemoryTool},
		resolvedToolInvocation{toolName: constants.SearchMemoryTool, command: "query"},
		map[string]string{"query": "golang"},
		"current-session",
		nil,
		&memoryUsed,
	)
	if result == nil || strings.TrimSpace(result.Error) != "" {
		t.Fatalf("expected search success, got %+v", result)
	}
	if !strings.Contains(result.Output, "Cross Session Golang") {
		t.Fatalf("expected cross-session result, got %q", result.Output)
	}
}

func TestBuildToolDefs_SortedByName(t *testing.T) {
	defs := BuildToolDefs()
	for i := 1; i < len(defs); i++ {
		if defs[i-1].Name > defs[i].Name {
			t.Fatalf("tool defs are not sorted: %q > %q", defs[i-1].Name, defs[i].Name)
		}
	}
}
