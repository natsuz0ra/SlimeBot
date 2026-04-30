package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"slimebot/internal/constants"
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

	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{}, constants.ApprovalModeStandard)
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

func TestResolveToolInvocation_RunSubagent(t *testing.T) {
	tc := llmsvc.ToolCallInfo{
		ID:        "call_sa",
		Name:      constants.RunSubagentTool,
		Arguments: `{"task":"Summarize X"}`,
	}
	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{}, constants.ApprovalModeStandard)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if invocation.toolName != constants.RunSubagentTool {
		t.Fatalf("unexpected toolName: %s", invocation.toolName)
	}
	if invocation.command != "run" {
		t.Fatalf("unexpected command: %s", invocation.command)
	}
	if invocation.requiresApproval {
		t.Fatal("run_subagent should not require approval")
	}
}

func TestResolveToolInvocation_SearchMemory(t *testing.T) {
	tc := llmsvc.ToolCallInfo{
		ID:        "call_2",
		Name:      constants.SearchMemoryTool,
		Arguments: `{"query":"golang"}`,
	}

	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{}, constants.ApprovalModeStandard)
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

func newTestMemoryService(t *testing.T) *memsvc.MemoryService {
	t.Helper()
	dir := t.TempDir()
	svc, err := memsvc.NewMemoryService(dir)
	if err != nil {
		t.Fatalf("create memory service: %v", err)
	}
	t.Cleanup(func() { svc.Shutdown(context.Background()) })
	return svc
}

func TestExecuteInvocation_SearchMemory_OncePerResponse(t *testing.T) {
	memorySvc := newTestMemoryService(t)

	// Pre-seed one memory row
	memorySvc.EnqueueTurnMemory("test-session", "", `{"name":"Golang Memory","description":"用户在聊 golang","type":"project","content":"用户喜欢 Go 语言"}`)

	agent := &AgentService{memory: memorySvc}

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

func TestBuildToolDefs_SortedByName(t *testing.T) {
	defs := BuildToolDefs()
	for i := 1; i < len(defs); i++ {
		if defs[i-1].Name > defs[i].Name {
			t.Fatalf("tool defs are not sorted: %q > %q", defs[i-1].Name, defs[i].Name)
		}
	}
}

func TestBuildToolDefs_ExecRunSchema(t *testing.T) {
	defs := BuildToolDefs()
	var execDef *llmsvc.ToolDef
	for i := range defs {
		if defs[i].Name == "exec__run" {
			execDef = &defs[i]
			break
		}
	}
	if execDef == nil {
		t.Fatal("expected exec__run tool definition")
	}

	params, ok := execDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected exec__run properties type: %#v", execDef.Parameters["properties"])
	}

	expected := []string{"command", "timeout_ms", "shell", "working_directory", "description"}
	for _, key := range expected {
		if _, found := params[key]; !found {
			t.Fatalf("expected exec__run param %q in tool schema", key)
		}
	}

	required, ok := execDef.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("unexpected required type: %#v", execDef.Parameters["required"])
	}
	if len(required) != 2 || required[0] != "command" || required[1] != "description" {
		t.Fatalf("expected required=[command description], got %#v", required)
	}
}

// Ensure underscore sanitization in names
var _ = filepath.Join
var _ = os.ReadFile
