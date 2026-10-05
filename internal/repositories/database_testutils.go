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
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: newGormSlogLogger(200 * time.Millisecond)})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&domain.Session{},
		&domain.Message{},
		&domain.ToolCallRecord{},
		&domain.ThinkingRecord{},
		&domain.TeamRun{},
		&domain.TeamMemberRun{},
		&domain.AgentDescriptor{}, &domain.AgentTurn{}, &domain.AgentInbox{}, &domain.AgentRootRequest{},
		&domain.AgentEvent{}, &domain.AgentTask{}, &domain.AgentArtifact{},
		&domain.AgentApproval{},
		&domain.AppSetting{},
		&domain.LLMProvider{},
		&domain.LLMConfig{},
		&domain.SessionContextSummary{},
		&domain.ContextEntry{},
		&domain.ContextHead{},
		&domain.ContextCheckpoint{},
		&domain.MCPConfig{},
		&domain.MessagePlatformConfig{},
		&domain.ScheduledTask{},
		&domain.ScheduledTaskRun{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return db
}
