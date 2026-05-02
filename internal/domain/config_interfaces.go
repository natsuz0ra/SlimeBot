package domain

import "context"

// LLMConfigStore persists LLM model configs.
type LLMConfigStore interface {
	ListLLMConfigs(ctx context.Context) ([]LLMConfig, error)
	CreateLLMConfig(ctx context.Context, item LLMConfig) (*LLMConfig, error)
	UpdateLLMConfig(ctx context.Context, id string, item LLMConfig) error
	DeleteLLMConfig(ctx context.Context, id string) error
}

// MCPConfigStore persists MCP server configs.
type MCPConfigStore interface {
	ListMCPConfigs(ctx context.Context) ([]MCPConfig, error)
	CreateMCPConfig(ctx context.Context, item MCPConfig) (*MCPConfig, error)
	UpdateMCPConfig(ctx context.Context, id string, item MCPConfig) error
	DeleteMCPConfig(ctx context.Context, id string) error
}

// MessagePlatformConfigStore persists message platform configs.
type MessagePlatformConfigStore interface {
	ListMessagePlatformConfigs(ctx context.Context) ([]MessagePlatformConfig, error)
	CreateMessagePlatformConfig(ctx context.Context, item MessagePlatformConfig) (*MessagePlatformConfig, error)
	UpdateMessagePlatformConfig(ctx context.Context, id string, item MessagePlatformConfig) error
	DeleteMessagePlatformConfig(ctx context.Context, id string) error
}
