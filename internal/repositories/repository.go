package repositories

import (
	"context"

	"slimebot/internal/domain"

	"gorm.io/gorm"
)

// Interface compliance checks at compile time.
var (
	_ domain.ChatStore                  = (*Repository)(nil)
	_ domain.SessionStore               = (*Repository)(nil)
	_ domain.LLMConfigStore             = (*Repository)(nil)
	_ domain.LLMProviderStore           = (*Repository)(nil)
	_ domain.MCPConfigStore             = (*Repository)(nil)
	_ domain.MessagePlatformConfigStore = (*Repository)(nil)
	_ domain.SettingsStore              = (*Repository)(nil)
	_ domain.TeamStore                  = (*Repository)(nil)
)

type Repository struct {
	db *gorm.DB
}

// New constructs a Repository.
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}

// Close releases the underlying database connection.
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
