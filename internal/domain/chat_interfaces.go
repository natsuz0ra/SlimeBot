package domain

import "context"

// ChatStore 聊天主流程数据访问接口。
type ChatStore interface {
	GetSessionByID(ctx context.Context, id string) (*Session, error)
	CreateSession(ctx context.Context, name string) (*Session, error)
	CreateSessionWithID(ctx context.Context, id, name string) (*Session, error)
	UpdateSessionTitle(ctx context.Context, id, name string) (bool, error)

	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	GetLLMConfigByID(ctx context.Context, id string) (*LLMConfig, error)
	ListLLMConfigs(ctx context.Context) ([]LLMConfig, error)

	ListRecentSessionMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)

	ListEnabledMCPConfigs(ctx context.Context) ([]MCPConfig, error)

	AddMessageWithInput(ctx context.Context, input AddMessageInput) (*Message, error)
	BindToolCallsToAssistantMessage(ctx context.Context, sessionID, requestID, assistantMessageID string) error

	UpsertToolCallStart(ctx context.Context, input ToolCallStartRecordInput) error
	UpdateToolCallResult(ctx context.Context, input ToolCallResultRecordInput) error
}
