package tools

import (
	"context"
	"strings"
)

// CommandParam describes one parameter for a tool command.
type CommandParam struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
	Schema      any    `json:"schema,omitempty"`
}

// Command describes one subcommand supported by a tool.
type Command struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Params      []CommandParam `json:"params,omitempty"`
}

// ExecuteResult is the outcome of a tool command.
type ExecuteResult struct {
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	Metadata any    `json:"metadata,omitempty"`
	// ImageURL is a data URL returned to the model after the matching tool result.
	ImageURL string `json:"-"`
}

// Tool is the interface every built-in tool implements.
// Add a new tool by implementing it in this package and calling Register from init().
type Tool interface {
	// Name returns the stable tool id (e.g. "exec", "http_request").
	Name() string
	// Description returns a short capability summary.
	Description() string
	// Commands lists supported subcommands.
	Commands() []Command
	Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error)
}

func IsStableNameTool(name string) bool {
	meta, ok := MetadataForTool(name)
	return ok && meta.StableName && meta.Name == strings.TrimSpace(name)
}

// IsPlanModeAllowedFunction reports whether a model-facing tool function is
// allowed while the agent is in read-only planning mode.
func IsPlanModeAllowedFunction(funcName string) bool {
	funcName = strings.TrimSpace(funcName)
	if funcName == "" || funcName == "todo__update" {
		return false
	}
	if meta, ok := MetadataForFunction(funcName); ok && meta.AllowedInPlanMode {
		return true
	}
	return false
}

func IsPlanModeAllowedCommand(toolName string) bool {
	meta, ok := MetadataForTool(toolName)
	return ok && meta.AllowedInPlanMode
}

func ParseFunctionName(funcName string) (toolName, command string, ok bool) {
	parts := strings.SplitN(funcName, "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
