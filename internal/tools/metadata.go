package tools

import (
	"strings"

	"slimebot/internal/constants"
)

// ToolMetadata describes model-facing and policy metadata for built-in tools.
type ToolMetadata struct {
	Name                 string
	StableName           bool
	DefaultCommand       string
	AllowedInPlanMode    bool
	ApprovalSensitive    bool
	HistoricalStableName bool
}

var metadataByTool = map[string]ToolMetadata{
	constants.ActivateSkillTool: {
		Name:                 constants.ActivateSkillTool,
		StableName:           true,
		DefaultCommand:       "activate",
		HistoricalStableName: true,
	},
	constants.RunSubagentTool: {
		Name:                 constants.RunSubagentTool,
		StableName:           true,
		DefaultCommand:       "run",
		AllowedInPlanMode:    true,
		HistoricalStableName: true,
	},
	constants.PlanStartTool: {
		Name:                 constants.PlanStartTool,
		AllowedInPlanMode:    true,
		HistoricalStableName: true,
	},
	constants.PlanCompleteTool: {
		Name:                 constants.PlanCompleteTool,
		AllowedInPlanMode:    true,
		HistoricalStableName: true,
	},
	"todo_update": {
		Name:              "todo",
		StableName:        true,
		DefaultCommand:    "update",
		AllowedInPlanMode: true,
	},
	constants.ExecToolName: {
		Name:              constants.ExecToolName,
		ApprovalSensitive: true,
	},
	constants.ScheduleToolName: {
		Name:              constants.ScheduleToolName,
		ApprovalSensitive: true,
	},
	"file_edit": {
		Name:              "file_edit",
		ApprovalSensitive: true,
	},
	"file_write": {
		Name:              "file_write",
		ApprovalSensitive: true,
	},
	"computer": {
		Name:              "computer",
		ApprovalSensitive: true,
	},
	constants.AskQuestionsTool: {
		Name: constants.AskQuestionsTool,
	},
	"web_search": {
		Name:              "web_search",
		AllowedInPlanMode: true,
	},
	"web_extract": {
		Name:              "web_extract",
		AllowedInPlanMode: true,
	},
	"file_read": {
		Name:              "file_read",
		AllowedInPlanMode: true,
	},
	"grep": {
		Name:              "grep",
		AllowedInPlanMode: true,
	},
	"glob": {
		Name:              "glob",
		AllowedInPlanMode: true,
	},
	"skills": {
		Name:              "skills",
		AllowedInPlanMode: true,
	},
	"todo": {
		Name:              "todo",
		AllowedInPlanMode: true,
	},
}

// MetadataForTool returns metadata for a tool id or stable model-facing function name.
func MetadataForTool(name string) (ToolMetadata, bool) {
	meta, ok := metadataByTool[strings.TrimSpace(name)]
	return meta, ok
}

// MetadataForFunction returns metadata for a model-facing function name.
func MetadataForFunction(funcName string) (ToolMetadata, bool) {
	funcName = strings.TrimSpace(funcName)
	if funcName == "" {
		return ToolMetadata{}, false
	}
	if meta, ok := MetadataForTool(funcName); ok && (meta.StableName || meta.HistoricalStableName) {
		return meta, true
	}
	toolName, _, ok := ParseFunctionName(funcName)
	if !ok {
		return ToolMetadata{}, false
	}
	return MetadataForTool(toolName)
}

func IsHistoricalStableName(name string) bool {
	meta, ok := MetadataForTool(name)
	return ok && meta.HistoricalStableName
}

func IsApprovalSensitiveTool(name string) bool {
	meta, ok := MetadataForTool(name)
	return ok && meta.ApprovalSensitive
}
