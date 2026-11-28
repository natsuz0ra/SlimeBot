package tools

import (
	"context"
	"fmt"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type runSubagentTool struct{}

func init() {
	Register(&runSubagentTool{})
}

func (r *runSubagentTool) Name() string { return constants.RunSubagentTool }

func (r *runSubagentTool) Description() string {
	return "[subagent] Delegate bounded, concise, independent sub-tasks to a nested agent with isolated context (no chat history). Prefer this only when separate focused research, codebase inspection, validation, or summarization has a clear stopping point. The parent agent remains responsible for integrating the result."
}

func (r *runSubagentTool) Commands() []Command {
	return []Command{
		{
			Name:        "run",
			Description: "Run one bounded, independent nested-agent task.",
			Params: []CommandParam{
				{Name: "title", Required: true, Description: "Short one-line title for the sub-agent task, about 80 characters or less."},
				{Name: "task", Required: true, Description: "Concrete self-contained task for the sub-agent, including the expected deliverable and boundaries."},
				{Name: "context", Description: "Optional compressed background from the main assistant; include only state the isolated sub-agent needs."},
			},
		},
	}
}

func (r *runSubagentTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "run":
		return r.run(ctx, params)
	default:
		return nil, fmt.Errorf("run_subagent tool does not support command: %s", command)
	}
}

func (r *runSubagentTool) run(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	runner, ok := subagentRunnerFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("subagent runner is unavailable")
	}
	task := paramStringTrim(params, "task")
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}
	request := SubagentRunRequest{
		Title:   paramStringTrim(params, "title"),
		Task:    task,
		Context: paramStringTrim(params, "context"),
		Params:  params,
	}
	return runner.RunSubagent(ctx, request)
}

// BuildRunSubagentToolDef builds the stable run_subagent definition.
func BuildRunSubagentToolDef() domain.ToolDef {
	return domain.ToolDef{
		Name:        constants.RunSubagentTool,
		Description: (&runSubagentTool{}).Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Concrete self-contained task for the sub-agent, including the expected deliverable and boundaries.",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Short one-line title for the sub-agent task, about 80 characters or less.",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional compressed background from the main assistant; include only state the isolated sub-agent needs.",
				},
			},
			"required": []string{"title", "task"},
		},
	}
}
