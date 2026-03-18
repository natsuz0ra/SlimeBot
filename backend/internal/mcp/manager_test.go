package mcp

import (
	"strings"
	"testing"

	"slimebot/backend/internal/consts"
)

func TestBuildMCPFuncName_LengthBounded(t *testing.T) {
	serverAlias := "mcp_" + strings.Repeat("a", 60)
	toolName := strings.Repeat("very_long_tool_name_", 5)

	name := buildMCPFuncName(serverAlias, toolName)
	if len(name) > consts.MCPFuncNameMaxLen {
		t.Fatalf("expected len <= %d, got %d: %s", consts.MCPFuncNameMaxLen, len(name), name)
	}
	if !strings.Contains(name, "__") {
		t.Fatalf("expected tool function name to contain separator '__': %s", name)
	}
}

func TestBuildMCPFuncName_StableAndDifferent(t *testing.T) {
	serverAlias := "mcp_server_123"

	nameA1 := buildMCPFuncName(serverAlias, "tool_alpha")
	nameA2 := buildMCPFuncName(serverAlias, "tool_alpha")
	if nameA1 != nameA2 {
		t.Fatalf("expected stable func name, got %s vs %s", nameA1, nameA2)
	}

	nameB := buildMCPFuncName(serverAlias, "tool_beta")
	if nameA1 == nameB {
		t.Fatalf("expected different tools to have different func names, both got %s", nameA1)
	}
}
