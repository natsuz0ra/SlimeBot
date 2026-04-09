package repositories

import (
	"context"

	"slimebot/internal/domain"

	"gorm.io/gorm"
)

// 接口合规性检查
var (
	_ domain.ChatStore                  = (*Repository)(nil)
	_ domain.SessionStore               = (*Repository)(nil)
	_ domain.LLMConfigStore             = (*Repository)(nil)
	_ domain.MCPConfigStore             = (*Repository)(nil)
	_ domain.MessagePlatformConfigStore = (*Repository)(nil)
	_ domain.SettingsStore              = (*Repository)(nil)
)

type Repository struct {
	db *gorm.DB
}

// New 创建 Repository 实例
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}
