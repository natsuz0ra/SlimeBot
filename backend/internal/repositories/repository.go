package repositories

import (
	"time"

	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

// SettingsReaderWriter 定义设置读写最小能力边界。
type SettingsReaderWriter interface {
	GetSetting(key string) (string, error)
	GetSettingBool(key string, defaultVal bool) (bool, error)
	SetSetting(key, value string) error
}

// SessionStore 定义会话及消息相关的数据访问能力。
type SessionStore interface {
	ListSessions() ([]models.Session, error)
	CreateSession(name string) (*models.Session, error)
	RenameSessionByUser(id, name string) error
	DeleteSession(id string) error
	ListSessionMessages(sessionID string) ([]models.Message, error)
	ListSessionMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]models.Message, bool, error)
	ListSessionToolCallRecords(sessionID string) ([]models.ToolCallRecord, error)
	SetSessionModel(sessionID, modelConfigID string) error
}

// LLMConfigStore 定义模型配置数据访问能力。
type LLMConfigStore interface {
	ListLLMConfigs() ([]models.LLMConfig, error)
	CreateLLMConfig(item models.LLMConfig) (*models.LLMConfig, error)
	DeleteLLMConfig(id string) error
}

// MCPConfigStore 定义 MCP 配置数据访问能力。
type MCPConfigStore interface {
	ListMCPConfigs() ([]models.MCPConfig, error)
	CreateMCPConfig(item models.MCPConfig) (*models.MCPConfig, error)
	UpdateMCPConfig(id string, item models.MCPConfig) error
	DeleteMCPConfig(id string) error
}

// MessagePlatformConfigStore 定义消息平台配置数据访问能力。
type MessagePlatformConfigStore interface {
	ListMessagePlatformConfigs() ([]models.MessagePlatformConfig, error)
	CreateMessagePlatformConfig(item models.MessagePlatformConfig) (*models.MessagePlatformConfig, error)
	UpdateMessagePlatformConfig(id string, item models.MessagePlatformConfig) error
	DeleteMessagePlatformConfig(id string) error
}

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}
