package tools

import "context"

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
