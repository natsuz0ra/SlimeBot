package repositories

import (
	"fmt"
	"path/filepath"
	"slimebot/backend/internal/domain"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func NewSQLite(dbPath string) (*gorm.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve database path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(absPath), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if err := db.AutoMigrate(
		&domain.Session{},
		&domain.Message{},
		&domain.SessionMemory{},
		&domain.ToolCallRecord{},
		&domain.AppSetting{},
		&domain.LLMConfig{},
		&domain.MCPConfig{},
		&domain.MessagePlatformConfig{},
		&domain.Skill{},
	); err != nil {
		return nil, fmt.Errorf("auto migration failed: %w", err)
	}

	return db, nil
}
