package domain

import (
	"context"
	"time"
)

type SettingsReaderWriter interface {
	GetSetting(key string) (string, error)
	GetSettingBool(key string, defaultVal bool) (bool, error)
	SetSetting(key, value string) error
}

type SettingsStore interface {
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

type SessionStore interface {
	ListSessions() ([]Session, error)
	CreateSession(name string) (*Session, error)
	RenameSessionByUser(id, name string) error
	DeleteSession(id string) error
	ListSessionMessages(sessionID string) ([]Message, error)
	ListSessionMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]Message, bool, error)
	ListSessionToolCallRecords(sessionID string) ([]ToolCallRecord, error)
	ListSessionToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]ToolCallRecord, error)
	SetSessionModel(sessionID, modelConfigID string) error
}

type LLMConfigStore interface {
	ListLLMConfigs() ([]LLMConfig, error)
	CreateLLMConfig(item LLMConfig) (*LLMConfig, error)
	DeleteLLMConfig(id string) error
}

type MCPConfigStore interface {
	ListMCPConfigs() ([]MCPConfig, error)
	CreateMCPConfig(item MCPConfig) (*MCPConfig, error)
	UpdateMCPConfig(id string, item MCPConfig) error
	DeleteMCPConfig(id string) error
}

type MessagePlatformConfigStore interface {
	ListMessagePlatformConfigs() ([]MessagePlatformConfig, error)
	CreateMessagePlatformConfig(item MessagePlatformConfig) (*MessagePlatformConfig, error)
	UpdateMessagePlatformConfig(id string, item MessagePlatformConfig) error
	DeleteMessagePlatformConfig(id string) error
}

type MemoryStore interface {
	CountSessionMessages(sessionID string) (int64, error)
	UpsertSessionMemoryIfNewer(input SessionMemoryUpsertInput) (bool, error)
	SearchMemoriesByKeywords(keywords []string, limit int, excludeSessionID string) ([]SessionMemorySearchHit, error)
	ListRecentSessionMessages(sessionID string, limit int) ([]Message, error)
	GetSessionMemory(sessionID string) (*SessionMemory, error)
	GetSessionMemoriesBySessionIDs(sessionIDs []string) ([]SessionMemory, error)
}

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

	ListEnabledMCPConfigsWithContext(ctx context.Context) ([]MCPConfig, error)

	AddMessageWithInputWithContext(ctx context.Context, input AddMessageInput) (*Message, error)
	BindToolCallsToAssistantMessageWithContext(ctx context.Context, sessionID, requestID, assistantMessageID string) error

	UpsertToolCallStartWithContext(ctx context.Context, input ToolCallStartRecordInput) error
	UpdateToolCallResultWithContext(ctx context.Context, input ToolCallResultRecordInput) error
}

type SkillStore interface {
	ListSkills() ([]Skill, error)
	GetSkillByName(name string) (*Skill, error)
	GetSkillByID(id string) (*Skill, error)
	CreateSkill(item Skill) (*Skill, error)
	DeleteSkill(id string) error
}
