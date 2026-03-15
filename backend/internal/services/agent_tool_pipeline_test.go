package services

import (
	"testing"

	"slimebot/backend/internal/mcp"
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
