package mcp

import "context"

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type CallResult struct {
	Output string
	Error  string
}

type Client interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (*CallResult, error)
	Close() error
}
