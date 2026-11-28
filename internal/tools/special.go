package tools

import "slimebot/internal/domain"

// SpecialToolOptions controls which stable-name special tools are exposed.
type SpecialToolOptions struct {
	Skills             []domain.Skill
	IncludeRunSubagent bool
	IncludePlanTools   bool
}

// BuildSpecialToolDefs returns stable-name special tool definitions in a fixed order.
func BuildSpecialToolDefs(opts SpecialToolOptions) []domain.ToolDef {
	var defs []domain.ToolDef
	if def := BuildActivateSkillToolDef(opts.Skills); def != nil {
		defs = append(defs, *def)
	}
	if opts.IncludeRunSubagent {
		defs = append(defs, BuildRunSubagentToolDef())
	}
	if opts.IncludePlanTools {
		defs = append(defs, BuildPlanToolDefs()...)
	}
	return defs
}
