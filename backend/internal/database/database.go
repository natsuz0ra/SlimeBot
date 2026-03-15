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
		return nil, fmt.Errorf("解析数据库路径失败: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(absPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	if err := db.AutoMigrate(
		&models.Session{},
		&models.Message{},
		&models.SessionMemory{},
		&models.ToolCallRecord{},
		&models.AppSetting{},
		&models.LLMConfig{},
		&models.MCPConfig{},
		&models.Skill{},
	); err != nil {
		return nil, fmt.Errorf("自动迁移失败: %w", err)
	}

	return db, nil
}
