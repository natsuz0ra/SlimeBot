package tools

import (
	"context"
	"fmt"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type activateSkillTool struct{}

func init() {
	Register(&activateSkillTool{})
}

func (a *activateSkillTool) Name() string { return constants.ActivateSkillTool }

func (a *activateSkillTool) Description() string {
	return "Load a skill guide by name. Call only when the task matches the skill description."
}

func (a *activateSkillTool) Commands() []Command {
	return []Command{
		{
			Name:        "activate",
			Description: "Load one enabled skill's full instructions into the current session.",
			Params: []CommandParam{
				{Name: "name", Required: true, Description: "Skill name or ID to activate.", Example: "imagegen"},
			},
		},
	}
}

func (a *activateSkillTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "activate":
		return a.activate(ctx, params)
	default:
		return nil, fmt.Errorf("activate_skill tool does not support command: %s", command)
	}
}

func (a *activateSkillTool) activate(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	runtime, ok := skillRuntimeFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("skill runtime is unavailable")
	}
	name := paramStringTrim(params, "name")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	content, _, err := runtime.ActivateSkill(name, activatedSkillsFromContext(ctx))
	if err != nil {
		return nil, err
	}
	return &ExecuteResult{Output: content}, nil
}

// BuildActivateSkillToolDef builds the dynamic activate_skill definition.
func BuildActivateSkillToolDef(skills []domain.Skill) *domain.ToolDef {
	if len(skills) == 0 {
		return nil
	}
	enumValues := make([]any, 0, len(skills))
	for _, item := range skills {
		if item.Enabled {
			enumValues = append(enumValues, item.ID)
		}
	}
	if len(enumValues) == 0 {
		return nil
	}
	return &domain.ToolDef{
		Name:        constants.ActivateSkillTool,
		Description: (&activateSkillTool{}).Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name or ID to activate.",
					"enum":        enumValues,
				},
			},
			"required": []string{"name"},
		},
	}
}
