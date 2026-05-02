package chat

import (
	"context"
	"strings"
	"testing"

	"slimebot/internal/constants"
	llmsvc "slimebot/internal/services/llm"
)

func TestBuildRuntimeToolDefs_IncludesRunSubagentAtDepth0Only(t *testing.T) {
	ctx := context.Background()
	agent := NewAgentService(nil, nil, nil)

	defs0, _, err := agent.buildRuntimeToolDefs(ctx, nil, 0)
	if err != nil {
		t.Fatalf("depth 0: %v", err)
	}
	if !containsToolName(defs0, constants.RunSubagentTool) {
		t.Fatal("expected run_subagent in defs at depth 0")
	}

	defs1, _, err := agent.buildRuntimeToolDefs(ctx, nil, 1)
	if err != nil {
		t.Fatalf("depth 1: %v", err)
	}
	if containsToolName(defs1, constants.RunSubagentTool) {
		t.Fatal("run_subagent must not appear at depth > 0")
	}
}

func TestRunSubagentToolDef_EncouragesBoundedDelegationWithIsolation(t *testing.T) {
	def := buildRunSubagentToolDef()
	desc := def.Description
	for _, want := range []string{
		"bounded",
		"concise",
		"independent",
		"isolated context",
	} {
		if !containsText(desc, want) {
			t.Fatalf("run_subagent description missing %q: %q", want, desc)
		}
	}
	for _, forbidden := range []string{"tool-heavy work", "proactively delegate"} {
		if containsText(desc, forbidden) {
			t.Fatalf("run_subagent description should not encourage %q: %q", forbidden, desc)
		}
	}

	properties, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("parameters.properties missing or invalid: %#v", def.Parameters["properties"])
	}
	if _, exists := properties["model_id"]; exists {
		t.Fatalf("run_subagent must not expose model_id to the LLM: %#v", properties["model_id"])
	}

	required, ok := def.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("required missing or invalid: %#v", def.Parameters["required"])
	}
	for _, want := range []string{"title", "task"} {
		if !containsString(required, want) {
			t.Fatalf("run_subagent required missing %q: %#v", want, required)
		}
	}

	title, ok := properties["title"].(map[string]any)
	if !ok {
		t.Fatalf("title property missing or invalid: %#v", properties["title"])
	}
	titleDesc, _ := title["description"].(string)
	for _, want := range []string{"Short", "title"} {
		if !containsText(titleDesc, want) {
			t.Fatalf("title description missing %q: %q", want, titleDesc)
		}
	}

	task, ok := properties["task"].(map[string]any)
	if !ok {
		t.Fatalf("task property missing or invalid: %#v", properties["task"])
	}
	taskDesc, _ := task["description"].(string)
	for _, want := range []string{"deliverable", "boundaries"} {
		if !containsText(taskDesc, want) {
			t.Fatalf("task description missing %q: %q", want, taskDesc)
		}
	}

	contextProp, ok := properties["context"].(map[string]any)
	if !ok {
		t.Fatalf("context property missing or invalid: %#v", properties["context"])
	}
	contextDesc, _ := contextProp["description"].(string)
	for _, want := range []string{"compressed", "background"} {
		if !containsText(contextDesc, want) {
			t.Fatalf("context description missing %q: %q", want, contextDesc)
		}
	}
}

func TestBuildRuntimeToolDefs_DoesNotExposeSearchMemory(t *testing.T) {
	ctx := context.Background()
	agent := NewAgentService(nil, nil, nil)

	defs, _, err := agent.buildRuntimeToolDefs(ctx, nil, 0)
	if err != nil {
		t.Fatalf("buildRuntimeToolDefs failed: %v", err)
	}
	legacyTool := "search" + "_" + "memory"
	if containsToolName(defs, legacyTool) {
		t.Fatalf("legacy memory tool should not be exposed after removal: %#v", toolNames(defs))
	}
}

func TestFilterPlanModeToolDefs_KeepsRunSubagentAndReadOnlyTools(t *testing.T) {
	defs := []llmsvc.ToolDef{
		{Name: constants.RunSubagentTool},
		{Name: "file_read__read"},
		{Name: "file_edit__edit"},
		{Name: "file_write__write"},
		{Name: "web_search__search"},
		{Name: constants.PlanStartTool},
		{Name: constants.PlanCompleteTool},
		{Name: "exec__run"},
		{Name: "http_request__request"},
	}

	filtered := filterPlanModeToolDefs(defs)

	for _, name := range []string{
		constants.RunSubagentTool,
		"file_read__read",
		"web_search__search",
		constants.PlanStartTool,
		constants.PlanCompleteTool,
	} {
		if !containsToolName(filtered, name) {
			t.Fatalf("expected plan mode to keep %s; got %#v", name, toolNames(filtered))
		}
	}
	for _, name := range []string{"exec__run", "file_edit__edit", "file_write__write", "http_request__request"} {
		if containsToolName(filtered, name) {
			t.Fatalf("expected plan mode to filter %s; got %#v", name, toolNames(filtered))
		}
	}
}

func containsToolName(defs []llmsvc.ToolDef, name string) bool {
	for _, d := range defs {
		if d.Name == name {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsText(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

func toolNames(defs []llmsvc.ToolDef) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}
