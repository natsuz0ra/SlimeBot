package testutil

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func NewSQLiteDB(t testing.TB, namespace string) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", namespace, time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
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
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}
