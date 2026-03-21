package domain

import (
	"context"
	"time"
)

// SettingsReaderWriter 设置读写（含布尔便捷方法）。
type SettingsReaderWriter interface {
	GetSetting(key string) (string, error)
	GetSettingBool(key string, defaultVal bool) (bool, error)
	SetSetting(key, value string) error
}

// SettingsStore 键值设置持久化。
type SettingsStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// SessionStore 会话 CRUD、消息列表/分页与工具调用记录查询。
type SessionStore interface {
	ListSessions(limit int, offset int, query string) ([]Session, error)
	CreateSession(name string) (*Session, error)
	RenameSessionByUser(id, name string) error
	DeleteSession(id string) error
	ListSessionMessages(sessionID string) ([]Message, error)
	ListSessionMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]Message, bool, error)
	ListSessionToolCallRecords(sessionID string) ([]ToolCallRecord, error)
	ListSessionToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]ToolCallRecord, error)
	SetSessionModel(sessionID, modelConfigID string) error
}

// LLMConfigStore LLM 配置列表与增删。
type LLMConfigStore interface {
	ListLLMConfigs() ([]LLMConfig, error)
	CreateLLMConfig(item LLMConfig) (*LLMConfig, error)
	DeleteLLMConfig(id string) error
}

// MCPConfigStore MCP 服务配置的 CRUD。
type MCPConfigStore interface {
	ListMCPConfigs() ([]MCPConfig, error)
	CreateMCPConfig(item MCPConfig) (*MCPConfig, error)
	UpdateMCPConfig(id string, item MCPConfig) error
	DeleteMCPConfig(id string) error
}

// MessagePlatformConfigStore 消息平台（如 Telegram）接入配置 CRUD。
type MessagePlatformConfigStore interface {
	ListMessagePlatformConfigs() ([]MessagePlatformConfig, error)
	CreateMessagePlatformConfig(item MessagePlatformConfig) (*MessagePlatformConfig, error)
	UpdateMessagePlatformConfig(id string, item MessagePlatformConfig) error
	DeleteMessagePlatformConfig(id string) error
}

// MemoryStore 记忆服务持久化：消息计数、会话记忆 upsert、关键词检索与按 ID 批量取记忆。
type MemoryStore interface {
	CountSessionMessages(sessionID string) (int64, error)
	UpsertSessionMemoryIfNewer(input SessionMemoryUpsertInput) (bool, error)
	SearchMemoriesByKeywords(keywords []string, limit int, excludeSessionID string) ([]SessionMemorySearchHit, error)
	ListRecentSessionMessages(sessionID string, limit int) ([]Message, error)
	GetSessionMemory(sessionID string) (*SessionMemory, error)
	GetSessionMemoriesBySessionIDs(sessionIDs []string) ([]SessionMemory, error)
	GetSessionMemoriesByIDs(ids []string) ([]SessionMemory, error)
	CountActiveSessionMemories(sessionID string) (int64, error)
	ListActiveSessionMemories(sessionID string) ([]SessionMemory, error)
	ListRecentActiveSessionMemories(sessionID string, limit int) ([]SessionMemory, error)
	CreateSessionMemory(input SessionMemoryCreateInput) (*SessionMemory, error)
	UpdateSessionMemoryContent(id, sessionID, summary string, keywords []string, sourceMessageCount int) error
	SoftDeleteSessionMemory(id, sessionID string) error
}

// ChatStore 聊天主流程数据访问：会话、设置、LLM、历史、记忆、MCP、消息与工具调用落库。
type ChatStore interface {
	GetSessionByIDWithContext(ctx context.Context, id string) (*Session, error)
	CreateSessionWithContext(ctx context.Context, name string) (*Session, error)
	CreateSessionWithIDWithContext(ctx context.Context, id, name string) (*Session, error)
	UpdateSessionTitleWithContext(ctx context.Context, id, name string) error

	GetSettingWithContext(ctx context.Context, key string) (string, error)
	SetSettingWithContext(ctx context.Context, key, value string) error

	GetLLMConfigByIDWithContext(ctx context.Context, id string) (*LLMConfig, error)
	ListLLMConfigsWithContext(ctx context.Context) ([]LLMConfig, error)

	ListRecentSessionMessagesWithContext(ctx context.Context, sessionID string, limit int) ([]Message, error)
	GetSessionMemoryWithContext(ctx context.Context, sessionID string) (*SessionMemory, error)
	ListActiveSessionMemoriesWithContext(ctx context.Context, sessionID string) ([]SessionMemory, error)

	ListEnabledMCPConfigsWithContext(ctx context.Context) ([]MCPConfig, error)

	AddMessageWithInputWithContext(ctx context.Context, input AddMessageInput) (*Message, error)
	BindToolCallsToAssistantMessageWithContext(ctx context.Context, sessionID, requestID, assistantMessageID string) error

	UpsertToolCallStartWithContext(ctx context.Context, input ToolCallStartRecordInput) error
	UpdateToolCallResultWithContext(ctx context.Context, input ToolCallResultRecordInput) error
}

// SkillStore 用户上传技能的列表与按名/ID 查询及增删。
type SkillStore interface {
	ListSkills() ([]Skill, error)
	GetSkillByName(name string) (*Skill, error)
	GetSkillByID(id string) (*Skill, error)
	CreateSkill(item Skill) (*Skill, error)
	DeleteSkill(id string) error
}
