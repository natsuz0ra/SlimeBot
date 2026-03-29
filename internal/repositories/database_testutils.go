package repositories

import (
	"fmt"
	"slimebot/internal/domain"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func NewSQLiteDBTest(t testing.TB, namespace string) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", namespace, time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&domain.Session{},
		&domain.Message{},
		&domain.EpisodeMemory{},
		&domain.StickyMemory{},
		&domain.ToolCallRecord{},
		&domain.AppSetting{},
		&domain.LLMConfig{},
		&domain.MCPConfig{},
		&domain.MessagePlatformConfig{},
		&domain.Skill{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}
