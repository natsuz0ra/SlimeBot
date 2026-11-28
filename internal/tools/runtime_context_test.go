package tools

import (
	"context"
	"maps"
	"testing"

	"slimebot/internal/sandbox"
)

func TestTurnToolRuntimeViewsKeepChildStateIsolated(t *testing.T) {
	policy, err := sandbox.DefaultPolicy(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := TurnToolRuntime{
		SessionID: "parent", SandboxPolicy: policy, TodoState: NewTodoState(),
		ActivatedSkills: map[string]struct{}{"parent-skill": {}},
	}
	runner := &captureSubagentRunner{}
	first := parent.WithActivatedSkills(parent.ActivatedSkills).WithSubagentRunner(runner)
	second := parent.WithActivatedSkills(parent.ActivatedSkills)
	first.ActivatedSkills["first-skill"] = struct{}{}
	second.ActivatedSkills["second-skill"] = struct{}{}

	ctx := WithTurnToolRuntime(context.Background(), parent)
	firstCtx := WithTurnToolRuntime(ctx, first)
	secondCtx := WithTurnToolRuntime(ctx, second)
	if !maps.Equal(activatedSkillsFromContext(ctx), map[string]struct{}{"parent-skill": {}}) ||
		!maps.Equal(activatedSkillsFromContext(firstCtx), map[string]struct{}{"parent-skill": {}, "first-skill": {}}) ||
		!maps.Equal(activatedSkillsFromContext(secondCtx), map[string]struct{}{"parent-skill": {}, "second-skill": {}}) {
		t.Fatal("skill activation leaked across runtime views")
	}
	if _, ok := subagentRunnerFromContext(ctx); ok {
		t.Fatal("child runner leaked into the parent runtime")
	}
	if got, ok := subagentRunnerFromContext(firstCtx); !ok || got != runner {
		t.Fatal("child runtime lost its runner")
	}
	if _, ok := subagentRunnerFromContext(secondCtx); ok {
		t.Fatal("child runner leaked into a sibling runtime")
	}
	if todoStateFromContext(firstCtx) != parent.TodoState || currentSessionIDFromContext(secondCtx) != "parent" {
		t.Fatal("isolated view lost shared session dependencies")
	}
	if got, ok := sandbox.FromContext(firstCtx); !ok || got != policy {
		t.Fatal("runtime sandbox policy did not reach the registered tools")
	}
}
