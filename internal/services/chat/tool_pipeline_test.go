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

func TestResolveToolInvocation_TodoUpdateAlias(t *testing.T) {
	tc := llmsvc.ToolCallInfo{
		ID:        "call_todo",
		Name:      todoUpdateFuncName,
		Arguments: `{"items":[{"id":"1","content":"Inspect","status":"in_progress"}]}`,
	}
	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{}, constants.ApprovalModeStandard)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if invocation.toolName != "todo" {
		t.Fatalf("unexpected toolName: %s", invocation.toolName)
	}
	if invocation.command != "update" {
		t.Fatalf("unexpected command: %s", invocation.command)
	}
}

func TestResolveToolInvocation_MCPUsesDisplayNamesAndKeepsExecutionMetadata(t *testing.T) {
	tc := llmsvc.ToolCallInfo{
		ID:        "call_mcp",
		Name:      "mcp_config_1__search_repositories",
		Arguments: `{"query":"slimebot"}`,
	}

	invocation, err := resolveToolInvocation(tc, map[string]mcp.ToolMeta{
		"mcp_config_1__search_repositories": {
			FuncName:    "mcp_config_1__search_repositories",
			ServerAlias: "mcp_config_1",
			ServerName:  "github",
			ToolName:    "search_repositories",
		},
	}, constants.ApprovalModeStandard)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if invocation.toolName != "github" {
		t.Fatalf("visible toolName = %q, want github", invocation.toolName)
	}
	if invocation.command != "search_repositories" {
		t.Fatalf("visible command = %q, want search_repositories", invocation.command)
	}
	if invocation.serverAlias != "mcp_config_1" {
		t.Fatalf("serverAlias = %q, want mcp_config_1", invocation.serverAlias)
	}
	if invocation.modelFuncName != "mcp_config_1__search_repositories" {
		t.Fatalf("modelFuncName = %q, want mcp_config_1__search_repositories", invocation.modelFuncName)
	}
	if !invocation.isMCP {
		t.Fatal("expected MCP invocation")
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

	expected := []string{"command", "timeout_ms", "shell", "working_directory", "description", "reason", "sandbox_permissions"}
	for _, key := range expected {
		if _, found := params[key]; !found {
			t.Fatalf("expected exec__run param %q in tool schema", key)
		}
	}

	required, ok := execDef.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("unexpected required type: %#v", execDef.Parameters["required"])
	}
	if len(required) != 1 || required[0] != "command" {
		t.Fatalf("expected required=[command], got %#v", required)
	}

	sandboxParam, ok := params["sandbox_permissions"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected sandbox_permissions schema: %#v", params["sandbox_permissions"])
	}
	enumValues, ok := sandboxParam["enum"].([]string)
	if !ok || !containsString(enumValues, "default") || !containsString(enumValues, "required_approval") {
		t.Fatalf("sandbox_permissions should expose controlled enum, got %#v", sandboxParam["enum"])
	}
}

func TestBuildToolDefs_FileToolSchemas(t *testing.T) {
	defs := BuildToolDefs()
	if containsToolName(defs, "todo__update") {
		t.Fatalf("todo__update should not be exposed by BuildToolDefs: %#v", toolNames(defs))
	}
	expected := map[string][]string{
		"file_read__read":      {},
		"file_edit__edit":      {},
		"file_write__write":    {},
		"glob__find":           {"pattern"},
		"grep__search":         {"pattern"},
		"process__list":        {},
		"process__status":      {},
		"process__stop":        {},
		"skills__list":         {},
		"skills__view":         {},
		"todo__list":           {},
		"todo_update":          {},
		"web_extract__extract": {},
	}
	for name, requiredParams := range expected {
		def := findToolDef(defs, name)
		if def == nil {
			t.Fatalf("expected %s tool definition", name)
		}
		properties, ok := def.Parameters["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s parameters.properties has unexpected type: %#v", name, def.Parameters["properties"])
		}
		required, _ := def.Parameters["required"].([]string)
		if len(requiredParams) > 0 && len(required) == 0 {
			t.Fatalf("%s should include required params", name)
		}
		for _, param := range requiredParams {
			if _, ok := properties[param]; !ok {
				t.Fatalf("%s missing property %q", name, param)
			}
			if !containsString(required, param) {
				t.Fatalf("%s missing required param %q in %#v", name, param, required)
			}
		}
	}
}

func TestBuildToolDefs_FileReadDescriptionPrefersBatchRanges(t *testing.T) {
	defs := BuildToolDefs()
	def := findToolDef(defs, "file_read__read")
	if def == nil {
		t.Fatal("expected file_read__read tool definition")
	}
	for _, want := range []string{
		"Prefer batch mode via requests[].ranges[]",
		"single-file mode (file_path/offset/limit)",
		"simple one-range reads",
	} {
		if !strings.Contains(def.Description, want) {
			t.Fatalf("file_read__read description missing %q: %q", want, def.Description)
		}
	}
}

func TestBuildToolDefs_GrepAndGlobSchemasExposePagination(t *testing.T) {
	defs := BuildToolDefs()
	grepDef := findToolDef(defs, "grep__search")
	if grepDef == nil {
		t.Fatal("expected grep__search tool definition")
	}
	if !strings.Contains(grepDef.Description, "use glob__find instead when the user is looking for a filename") {
		t.Fatalf("grep__search description should steer filename searches to glob: %q", grepDef.Description)
	}
	grepProps, ok := grepDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("grep parameters.properties has unexpected type: %#v", grepDef.Parameters["properties"])
	}
	for _, prop := range []string{"pattern", "output_mode", "head_limit", "offset"} {
		if _, ok := grepProps[prop]; !ok {
			t.Fatalf("grep__search missing %s property: %#v", prop, grepProps)
		}
	}

	globDef := findToolDef(defs, "glob__find")
	if globDef == nil {
		t.Fatal("expected glob__find tool definition")
	}
	if !strings.Contains(globDef.Description, "set path to ~/Downloads") {
		t.Fatalf("glob__find description should mention Downloads path handling: %q", globDef.Description)
	}
	globProps, ok := globDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("glob parameters.properties has unexpected type: %#v", globDef.Parameters["properties"])
	}
	for _, prop := range []string{"pattern", "limit", "offset"} {
		if _, ok := globProps[prop]; !ok {
			t.Fatalf("glob__find missing %s property: %#v", prop, globProps)
		}
	}
}

func TestRequiresToolApproval_FileWritesInStandardMode(t *testing.T) {
	if !requiresToolApproval("file_edit", false, constants.ApprovalModeStandard) {
		t.Fatal("file_edit should require approval in standard mode")
	}
	if !requiresToolApproval("file_write", false, constants.ApprovalModeStandard) {
		t.Fatal("file_write should require approval in standard mode")
	}
	if requiresToolApproval("file_read", false, constants.ApprovalModeStandard) {
		t.Fatal("file_read should not require approval in standard mode")
	}
	if requiresToolApproval("file_write", false, constants.ApprovalModeAuto) {
		t.Fatal("file_write should not require approval in auto mode")
	}
}

func TestDetermineToolApprovalPolicy_AutoReviewForSensitiveBuiltins(t *testing.T) {
	cases := []struct {
		name         string
		toolName     string
		isMCP        bool
		approvalMode string
		want         toolApprovalPolicy
	}{
		{name: "exec standard", toolName: constants.ExecToolName, approvalMode: constants.ApprovalModeStandard, want: toolApprovalPolicyManual},
		{name: "file edit auto review", toolName: "file_edit", approvalMode: constants.ApprovalModeAutoReview, want: toolApprovalPolicyAutoReview},
		{name: "file write auto review", toolName: "file_write", approvalMode: constants.ApprovalModeAutoReview, want: toolApprovalPolicyAutoReview},
		{name: "file read auto review", toolName: "file_read", approvalMode: constants.ApprovalModeAutoReview, want: toolApprovalPolicyNone},
		{name: "mcp auto review requires review", toolName: "github", isMCP: true, approvalMode: constants.ApprovalModeAutoReview, want: toolApprovalPolicyAutoReview},
		{name: "mcp auto still requires manual", toolName: "github", isMCP: true, approvalMode: constants.ApprovalModeAuto, want: toolApprovalPolicyManual},
		{name: "mcp scheduled auto skips approval", toolName: "github", isMCP: true, approvalMode: constants.ApprovalModeScheduledAuto, want: toolApprovalPolicyNone},
		{name: "exec auto execute", toolName: constants.ExecToolName, approvalMode: constants.ApprovalModeAuto, want: toolApprovalPolicyNone},
		{name: "ask questions always manual", toolName: constants.AskQuestionsTool, approvalMode: constants.ApprovalModeAutoReview, want: toolApprovalPolicyManual},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := determineToolApprovalPolicy(tc.toolName, tc.isMCP, tc.approvalMode)
			if got != tc.want {
				t.Fatalf("policy = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestApplyParamApprovalPolicy_RequiredApprovalForcesManual(t *testing.T) {
	invocation := resolvedToolInvocation{
		toolName:         constants.ExecToolName,
		command:          "run",
		requiresApproval: false,
		approvalPolicy:   toolApprovalPolicyNone,
	}
	got := applyParamApprovalPolicy(invocation, map[string]any{"sandbox_permissions": "required_approval"})
	if !got.requiresApproval {
		t.Fatal("required_approval should force approval")
	}
	if got.approvalPolicy != toolApprovalPolicyManual {
		t.Fatalf("approval policy = %s, want manual", got.approvalPolicy)
	}
}

func TestWaitApprovalIfNeeded_AutoReviewApproveSkipsUserApproval(t *testing.T) {
	tc := llmsvc.ToolCallInfo{ID: "call-review", Name: "exec__run", Arguments: `{"command":"date"}`}
	invocation := resolvedToolInvocation{
		toolName:         constants.ExecToolName,
		command:          "run",
		requiresApproval: true,
		approvalPolicy:   toolApprovalPolicyAutoReview,
	}
	var waited bool
	var statuses []string
	approved, rejectionMessage, _ := waitApprovalIfNeeded(context.Background(), AgentCallbacks{
		WaitApproval: func(context.Context, string) (*ApprovalResponse, error) {
			waited = true
			return &ApprovalResponse{ToolCallID: tc.ID, Approved: true}, nil
		},
		OnToolApprovalReview: func(event ApprovalReviewEvent) error {
			statuses = append(statuses, event.ReviewStatus)
			return nil
		},
	}, tc, invocation, map[string]any{"command": "date"}, "", func(ctx context.Context, req ApprovalReviewRequest) (*ApprovalReviewResult, error) {
		return &ApprovalReviewResult{Decision: ApprovalReviewDecisionApprove, Risk: "low", Reason: "routine read-only command"}, nil
	})

	if !approved || rejectionMessage != "" {
		t.Fatalf("expected auto-review approval, approved=%v rejection=%q", approved, rejectionMessage)
	}
	if waited {
		t.Fatal("auto-review approve should not wait for user approval")
	}
	if strings.Join(statuses, ",") != "reviewing,approved" {
		t.Fatalf("unexpected review statuses: %v", statuses)
	}
}

func TestWaitApprovalIfNeeded_AutoReviewAskUserFallsBackToManualApproval(t *testing.T) {
	tc := llmsvc.ToolCallInfo{ID: "call-review", Name: "exec__run", Arguments: `{"command":"rm -rf /tmp/example"}`}
	invocation := resolvedToolInvocation{
		toolName:         constants.ExecToolName,
		command:          "run",
		requiresApproval: true,
		approvalPolicy:   toolApprovalPolicyAutoReview,
	}
	var required bool
	var waited bool
	var statuses []string
	approved, rejectionMessage, _ := waitApprovalIfNeeded(context.Background(), AgentCallbacks{
		OnToolApprovalRequired: func(req ApprovalRequest) error {
			required = req.RequiresApproval
			return nil
		},
		WaitApproval: func(context.Context, string) (*ApprovalResponse, error) {
			waited = true
			return &ApprovalResponse{ToolCallID: tc.ID, Approved: true}, nil
		},
		OnToolApprovalReview: func(event ApprovalReviewEvent) error {
			statuses = append(statuses, event.ReviewStatus)
			return nil
		},
	}, tc, invocation, map[string]any{"command": "rm -rf /tmp/example"}, "", func(ctx context.Context, req ApprovalReviewRequest) (*ApprovalReviewResult, error) {
		return &ApprovalReviewResult{Decision: ApprovalReviewDecisionAskUser, Risk: "high", Reason: "destructive command"}, nil
	})

	if !approved || rejectionMessage != "" {
		t.Fatalf("expected manual approval after review fallback, approved=%v rejection=%q", approved, rejectionMessage)
	}
	if !required || !waited {
		t.Fatalf("expected fallback approval prompt and wait, required=%v waited=%v", required, waited)
	}
	if strings.Join(statuses, ",") != "reviewing,needs_user" {
		t.Fatalf("unexpected review statuses: %v", statuses)
	}
}

func TestNormalizeApprovalReviewResult_ForcesUnclearOrHighRiskToAskUser(t *testing.T) {
	for _, risk := range []string{"high", "critical", "unknown"} {
		t.Run(risk, func(t *testing.T) {
			result := normalizeApprovalReviewResult(&ApprovalReviewResult{
				Decision: ApprovalReviewDecisionApprove,
				Risk:     risk,
				Reason:   "model tried to approve risky action",
			})
			if result.Decision != ApprovalReviewDecisionAskUser {
				t.Fatalf("decision = %s, want %s", result.Decision, ApprovalReviewDecisionAskUser)
			}
		})
	}
}

func TestBuildApprovalReviewPromptIncludesExecAuditContext(t *testing.T) {
	prompt := buildApprovalReviewPrompt([]llmsvc.ChatMessage{
		{Role: "user", Content: "Please inspect the Go version."},
	}, ApprovalReviewRequest{
		ToolCallID:       "call-exec",
		ToolName:         constants.ExecToolName,
		Command:          "run",
		WorkingDirectory: "/tmp/session-project",
		Params: map[string]any{
			"command":             "go version",
			"description":         "Check Go version for a delegated task",
			"reason":              "Subagent needs to verify toolchain availability",
			"working_directory":   "/tmp/example",
			"sandbox_permissions": "required_approval",
		},
	})

	for _, want := range []string{
		`"command": "run"`,
		`"command": "go version"`,
		`"description": "Check Go version for a delegated task"`,
		`"reason": "Subagent needs to verify toolchain availability"`,
		`"working_directory": "/tmp/example"`,
		`"sandbox_permissions": "required_approval"`,
		`"currentWorkingDirectory": "/tmp/session-project"`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("approval review prompt missing %q:\n%s", want, prompt)
		}
	}
}

func findToolDef(defs []llmsvc.ToolDef, name string) *llmsvc.ToolDef {
	for i := range defs {
		if defs[i].Name == name {
			return &defs[i]
		}
	}
	return nil
}

// Ensure underscore sanitization in names
var _ = filepath.Join
var _ = os.ReadFile
