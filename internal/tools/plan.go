package tools

import (
	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// BuildPlanToolDefs returns the stable plan-mode control tools in call order.
func BuildPlanToolDefs() []domain.ToolDef {
	return []domain.ToolDef{
		BuildPlanStartToolDef(),
		BuildPlanCompleteToolDef(),
	}
}

// BuildPlanStartToolDef builds the plan_start control tool definition.
func BuildPlanStartToolDef() domain.ToolDef {
	return domain.ToolDef{
		Name:        constants.PlanStartTool,
		Description: "[plan] Call this tool when you are ready to begin writing your plan. All text output BEFORE this call will appear as narration; all text AFTER will be the plan body. You MUST call this before writing your plan.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

// BuildPlanCompleteToolDef builds the plan_complete__submit control tool definition.
func BuildPlanCompleteToolDef() domain.ToolDef {
	return domain.ToolDef{
		Name:        constants.PlanCompleteTool,
		Description: "[plan] Call this tool ONLY when your complete plan has been written in your response. This submits the plan for user review. You MUST call this tool when you finish writing your plan — without it the user will not see the review menu.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Short title for the plan. Omit to auto-detect from the first heading.",
				},
			},
		},
	}
}
