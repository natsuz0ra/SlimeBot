package tools

import (
	"context"
	"strings"
	"testing"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	skillsvc "slimebot/internal/services/skill"
)

func TestActivateSkillToolActivatesSkillFromRuntimeContext(t *testing.T) {
	root := t.TempDir()
	writeToolSkill(t, root, "alpha", "Alpha skill", "Alpha body")
	runtime := skillsvc.NewSkillRuntimeService(skillsvc.NewFileSystemSkillStore(root), root)
	activated := map[string]struct{}{}
	ctx := WithActivatedSkills(WithSkillRuntime(context.Background(), runtime), activated)

	result, err := (&activateSkillTool{}).Execute(ctx, "activate", map[string]any{"name": "alpha"})
	if err != nil {
		t.Fatalf("activate failed: %v", err)
	}
	if !strings.Contains(result.Output, "Alpha body") {
		t.Fatalf("unexpected activation output:\n%s", result.Output)
	}
	if _, ok := activated["alpha"]; !ok {
		t.Fatalf("expected activated skill state to include alpha")
	}
}

func TestActivateSkillToolRequiresRuntimeContext(t *testing.T) {
	_, err := (&activateSkillTool{}).Execute(context.Background(), "activate", map[string]any{"name": "alpha"})
	if err == nil || !strings.Contains(err.Error(), "skill runtime is unavailable") {
		t.Fatalf("expected missing runtime error, got %v", err)
	}
}

func TestRunSubagentToolDelegatesToContextRunner(t *testing.T) {
	runner := &captureSubagentRunner{}
	ctx := WithSubagentRunner(context.Background(), runner)

	result, err := (&runSubagentTool{}).Execute(ctx, "run", map[string]any{
		"title":   "Inspect",
		"task":    "inspect the code",
		"context": "repo background",
	})
	if err != nil {
		t.Fatalf("run_subagent failed: %v", err)
	}
	if result.Output != "subagent result" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if runner.request.Title != "Inspect" || runner.request.Task != "inspect the code" || runner.request.Context != "repo background" {
		t.Fatalf("unexpected runner request: %+v", runner.request)
	}
}

func TestRunSubagentToolRequiresRunnerAndTask(t *testing.T) {
	_, err := (&runSubagentTool{}).Execute(context.Background(), "run", map[string]any{"task": "inspect"})
	if err == nil || !strings.Contains(err.Error(), "subagent runner is unavailable") {
		t.Fatalf("expected missing runner error, got %v", err)
	}

	ctx := WithSubagentRunner(context.Background(), &captureSubagentRunner{})
	_, err = (&runSubagentTool{}).Execute(ctx, "run", map[string]any{"title": "Inspect"})
	if err == nil || !strings.Contains(err.Error(), "task is required") {
		t.Fatalf("expected missing task error, got %v", err)
	}
}

func TestBuildPlanToolDefs(t *testing.T) {
	defs := BuildPlanToolDefs()
	if len(defs) != 2 {
		t.Fatalf("expected two plan tool defs, got %d: %#v", len(defs), defs)
	}
	if defs[0].Name != constants.PlanStartTool {
		t.Fatalf("expected first plan tool to be %s, got %s", constants.PlanStartTool, defs[0].Name)
	}
	if defs[1].Name != constants.PlanCompleteTool {
		t.Fatalf("expected second plan tool to be %s, got %s", constants.PlanCompleteTool, defs[1].Name)
	}

	startParams := defs[0].Parameters
	if startParams["type"] != "object" {
		t.Fatalf("expected plan_start object schema, got %#v", startParams["type"])
	}
	startProps, ok := startParams["properties"].(map[string]any)
	if !ok || len(startProps) != 0 {
		t.Fatalf("expected plan_start empty properties, got %#v", startParams["properties"])
	}

	completeParams := defs[1].Parameters
	completeProps, ok := completeParams["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected plan_complete properties, got %#v", completeParams["properties"])
	}
	titleProp, ok := completeProps["title"].(map[string]any)
	if !ok {
		t.Fatalf("expected plan_complete title property, got %#v", completeProps["title"])
	}
	if titleProp["type"] != "string" {
		t.Fatalf("expected title property to be string, got %#v", titleProp["type"])
	}
	if _, ok := completeParams["required"]; ok {
		t.Fatalf("plan_complete title should remain optional, got required=%#v", completeParams["required"])
	}
}

func TestPlanToolsAllowedInPlanMode(t *testing.T) {
	if !IsPlanModeAllowedFunction(constants.PlanStartTool) {
		t.Fatalf("expected %s to be allowed in plan mode", constants.PlanStartTool)
	}
	if !IsPlanModeAllowedFunction(constants.PlanCompleteTool) {
		t.Fatalf("expected %s to be allowed in plan mode", constants.PlanCompleteTool)
	}
}

func TestBuildSpecialToolDefsIncludesOnlyRequestedStableTools(t *testing.T) {
	defs := BuildSpecialToolDefs(SpecialToolOptions{
		Skills: []domain.Skill{
			{ID: "disabled", Enabled: false},
			{ID: "alpha", Enabled: true},
		},
		IncludeRunSubagent: true,
		IncludePlanTools:   true,
	})

	wantNames := []string{
		constants.ActivateSkillTool,
		constants.RunSubagentTool,
		constants.PlanStartTool,
		constants.PlanCompleteTool,
	}
	if len(defs) != len(wantNames) {
		t.Fatalf("expected %d special defs, got %d: %#v", len(wantNames), len(defs), defs)
	}
	for i, want := range wantNames {
		if defs[i].Name != want {
			t.Fatalf("expected special def %d to be %s, got %s", i, want, defs[i].Name)
		}
	}
	if containsToolDefName(defs, "activate_skill__activate") {
		t.Fatalf("activate_skill must only be exposed as stable name: %#v", specialToolNames(defs))
	}
	if containsToolDefName(defs, "run_subagent__run") {
		t.Fatalf("run_subagent must only be exposed as stable name: %#v", specialToolNames(defs))
	}
}

func TestBuildSpecialToolDefsRespectsVisibilityOptions(t *testing.T) {
	defs := BuildSpecialToolDefs(SpecialToolOptions{
		Skills:             []domain.Skill{{ID: "disabled", Enabled: false}},
		IncludeRunSubagent: false,
		IncludePlanTools:   false,
	})
	if len(defs) != 0 {
		t.Fatalf("expected no special defs, got %#v", specialToolNames(defs))
	}

	defs = BuildSpecialToolDefs(SpecialToolOptions{IncludeRunSubagent: true})
	if len(defs) != 1 || defs[0].Name != constants.RunSubagentTool {
		t.Fatalf("expected only run_subagent, got %#v", specialToolNames(defs))
	}
}

func TestMetadataForFunctionResolvesStableSpecialTools(t *testing.T) {
	cases := []struct {
		funcName string
		toolName string
		command  string
		stable   bool
	}{
		{funcName: constants.ActivateSkillTool, toolName: constants.ActivateSkillTool, command: "activate", stable: true},
		{funcName: constants.RunSubagentTool, toolName: constants.RunSubagentTool, command: "run", stable: true},
		{funcName: "todo_update", toolName: "todo", command: "update", stable: true},
	}

	for _, tc := range cases {
		t.Run(tc.funcName, func(t *testing.T) {
			meta, ok := MetadataForFunction(tc.funcName)
			if !ok {
				t.Fatalf("expected metadata for %s", tc.funcName)
			}
			if meta.Name != tc.toolName || meta.DefaultCommand != tc.command || meta.StableName != tc.stable {
				t.Fatalf("unexpected metadata for %s: %+v", tc.funcName, meta)
			}
		})
	}
}

func TestToolMetadataCentralizesPlanAndApprovalRules(t *testing.T) {
	if IsPlanModeAllowedFunction("todo__update") {
		t.Fatal("todo__update should remain blocked in plan mode")
	}
	for _, name := range []string{constants.PlanStartTool, constants.PlanCompleteTool} {
		meta, ok := MetadataForFunction(name)
		if !ok {
			t.Fatalf("expected metadata for %s", name)
		}
		if !meta.AllowedInPlanMode || !meta.HistoricalStableName {
			t.Fatalf("expected %s to be plan-allowed and historical-stable, got %+v", name, meta)
		}
	}
	for _, name := range []string{constants.ExecToolName, "file_edit", "file_write"} {
		if !IsApprovalSensitiveTool(name) {
			t.Fatalf("expected %s to be approval-sensitive", name)
		}
	}
	if IsApprovalSensitiveTool("file_read") {
		t.Fatal("file_read should not be approval-sensitive")
	}
}

func TestModelFunctionNameHandlesTodoAlias(t *testing.T) {
	if got := ModelFunctionName("todo", "update"); got != TodoUpdateFunctionName {
		t.Fatalf("todo update function name = %s, want %s", got, TodoUpdateFunctionName)
	}
	if got := ModelFunctionName("file_read", "read"); got != "file_read__read" {
		t.Fatalf("file_read function name = %s, want file_read__read", got)
	}
}

func TestBuildRegistryToolDefsAliasesAndSkipsStableSpecialTools(t *testing.T) {
	defs := BuildRegistryToolDefs()
	if containsToolDefName(defs, "todo__update") {
		t.Fatalf("todo__update should not be exposed: %#v", specialToolNames(defs))
	}
	for _, name := range []string{TodoUpdateFunctionName, "exec__run", "file_read__read", "grep__search", "glob__find"} {
		if !containsToolDefName(defs, name) {
			t.Fatalf("expected registry tool %s in %#v", name, specialToolNames(defs))
		}
	}
	if containsToolDefName(defs, "search_files__search") {
		t.Fatalf("search_files__search should not be exposed: %#v", specialToolNames(defs))
	}
	for _, name := range []string{"activate_skill__activate", "run_subagent__run"} {
		if containsToolDefName(defs, name) {
			t.Fatalf("stable special tool %s must not be exposed: %#v", name, specialToolNames(defs))
		}
	}
	for i := 1; i < len(defs); i++ {
		if defs[i-1].Name > defs[i].Name {
			t.Fatalf("registry tool defs are not sorted: %q > %q", defs[i-1].Name, defs[i].Name)
		}
	}
}

func TestBuildRegistryToolDefsPreservesKeySchemas(t *testing.T) {
	defs := BuildRegistryToolDefs()
	execDef := findSpecialToolDef(defs, "exec__run")
	if execDef == nil {
		t.Fatal("expected exec__run tool definition")
	}
	execProps, ok := execDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("exec__run parameters.properties has unexpected type: %#v", execDef.Parameters["properties"])
	}
	for _, name := range []string{"command", "timeout_ms", "shell", "working_directory", "description", "reason", "sandbox_permissions"} {
		if _, ok := execProps[name]; !ok {
			t.Fatalf("exec__run missing property %q", name)
		}
	}
	required, ok := execDef.Parameters["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "command" {
		t.Fatalf("exec__run required = %#v, want [command]", execDef.Parameters["required"])
	}

	grepDef := findSpecialToolDef(defs, "grep__search")
	if grepDef == nil {
		t.Fatal("expected grep__search tool definition")
	}
	grepProps, ok := grepDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("grep parameters.properties has unexpected type: %#v", grepDef.Parameters["properties"])
	}
	if _, ok := grepProps["head_limit"]; !ok {
		t.Fatalf("grep__search missing head_limit property: %#v", grepProps)
	}

	globDef := findSpecialToolDef(defs, "glob__find")
	if globDef == nil {
		t.Fatal("expected glob__find tool definition")
	}
	globProps, ok := globDef.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("glob parameters.properties has unexpected type: %#v", globDef.Parameters["properties"])
	}
	if _, ok := globProps["limit"]; !ok {
		t.Fatalf("glob__find missing limit property: %#v", globProps)
	}
}

func containsToolDefName(defs []domain.ToolDef, name string) bool {
	for _, def := range defs {
		if def.Name == name {
			return true
		}
	}
	return false
}

func specialToolNames(defs []domain.ToolDef) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

func findSpecialToolDef(defs []domain.ToolDef, name string) *domain.ToolDef {
	for i := range defs {
		if defs[i].Name == name {
			return &defs[i]
		}
	}
	return nil
}

type captureSubagentRunner struct {
	request SubagentRunRequest
}

func (r *captureSubagentRunner) RunSubagent(ctx context.Context, request SubagentRunRequest) (*ExecuteResult, error) {
	_ = ctx
	r.request = request
	return &ExecuteResult{Output: "subagent result"}, nil
}
