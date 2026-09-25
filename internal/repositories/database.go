package repositories

import (
	"fmt"
	"path/filepath"
	"slimebot/internal/domain"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func NewSQLite(dbPath string) (*gorm.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve database path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(absPath), &gorm.Config{
		PrepareStmt: true,
		Logger:      newGormSlogLogger(200 * time.Millisecond),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if err := db.AutoMigrate(
		&domain.Session{},
		&domain.Message{},
		&domain.ToolCallRecord{},
		&domain.ThinkingRecord{},
		&domain.TeamRun{},
		&domain.TeamMemberRun{},
		&domain.AppSetting{},
		&domain.LLMProvider{},
		&domain.LLMConfig{},
		&domain.SessionContextSummary{},
		&domain.MCPConfig{},
		&domain.MessagePlatformConfig{},
		&domain.ScheduledTask{},
		&domain.ScheduledTaskRun{},
	); err != nil {
		return nil, fmt.Errorf("auto migration failed: %w", err)
	}
	if err := migrateLegacyLLMProviders(db); err != nil {
		return nil, fmt.Errorf("LLM provider migration failed: %w", err)
	}

	return db, nil
}
