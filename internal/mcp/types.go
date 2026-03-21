package mcp

import "context"

// Tool 描述 MCP 服务暴露的单个工具定义。
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// CallResult 描述一次 MCP 工具调用结果。
type CallResult struct {
	Output string
	Error  string
}

// Client 定义 MCP 传输层客户端需要实现的统一能力。
type Client interface {
	// ListTools 返回服务当前可用的工具列表。
	ListTools(ctx context.Context) ([]Tool, error)
	// CallTool 按工具名发起调用并返回标准化结果。
	CallTool(ctx context.Context, name string, arguments map[string]any) (*CallResult, error)
	// Close 释放底层连接或进程资源。
	Close() error
}
