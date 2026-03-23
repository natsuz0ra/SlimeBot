package domain

import "context"

// LLMConfigStore LLM 配置存储接口。
type LLMConfigStore interface {
	ListLLMConfigs(ctx context.Context) ([]LLMConfig, error)
	CreateLLMConfig(item LLMConfig) (*LLMConfig, error)
	DeleteLLMConfig(id string) error
}

// MCPConfigStore MCP 服务配置存储接口。
type MCPConfigStore interface {
	ListMCPConfigs() ([]MCPConfig, error)
	CreateMCPConfig(item MCPConfig) (*MCPConfig, error)
	UpdateMCPConfig(id string, item MCPConfig) error
	DeleteMCPConfig(id string) error
}

// MessagePlatformConfigStore 消息平台配置存储接口。
type MessagePlatformConfigStore interface {
	ListMessagePlatformConfigs() ([]MessagePlatformConfig, error)
	CreateMessagePlatformConfig(item MessagePlatformConfig) (*MessagePlatformConfig, error)
	UpdateMessagePlatformConfig(id string, item MessagePlatformConfig) error
	DeleteMessagePlatformConfig(id string) error
}
