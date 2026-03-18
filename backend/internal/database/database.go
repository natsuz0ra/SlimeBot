package database

import (
	"fmt"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func NewSQLite(dbPath string) (*gorm.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve database path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(absPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if err := db.AutoMigrate(
		&models.Session{},
		&models.Message{},
		&models.SessionMemory{},
		&models.ToolCallRecord{},
		&models.AppSetting{},
		&models.LLMConfig{},
		&models.MCPConfig{},
		&models.MessagePlatformConfig{},
		&models.Skill{},
	); err != nil {
		return nil, fmt.Errorf("auto migration failed: %w", err)
	}

	return db, nil
}
