package repositories

import (
	"fmt"
	"time"

	"slimebot/internal/domain"

	"gorm.io/gorm"
)

// SchemaMigration records one applied, forward-only data migration.
type SchemaMigration struct {
	Version   int       `gorm:"primaryKey"`
	Name      string    `gorm:"size:128;not null"`
	AppliedAt time.Time `gorm:"not null"`
}

// AllModels is the single schema manifest shared by production and tests.
func AllModels() []any {
	return []any{
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
		&domain.ContextCalibration{},
		&domain.MCPConfig{},
		&domain.ACPConfig{},
		&domain.MessagePlatformConfig{},
		&domain.ScheduledTask{},
		&domain.ScheduledTaskRun{},
		&domain.BackgroundTask{},
		&domain.UsageRecord{},
	}
}

type schemaMigrationStep struct {
	version int
	name    string
	apply   func(*gorm.DB) error
}

var schemaMigrationSteps = []schemaMigrationStep{
	{version: 1, name: "interrupt_pending_checkpoints", apply: recoverPendingContextCheckpoints},
	// NewSQLite also invokes checkpoint recovery on every startup. Version 1
	// upgrades legacy databases; it is not a substitute for crash recovery.
	{version: 2, name: "migrate_scheduled_run_contexts", apply: migrateScheduledRunContexts},
	{version: 3, name: "migrate_legacy_llm_providers", apply: migrateLegacyLLMProviders},
}

// RunMigrations synchronizes the current schema and applies each unrecorded
// data migration in version order. Applied migrations are never run backwards.
func RunMigrations(db *gorm.DB) error {
	models := append(AllModels(), &SchemaMigration{})
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("auto migration failed: %w", err)
	}
	applied := make(map[int]struct{})
	var records []SchemaMigration
	if err := db.Find(&records).Error; err != nil {
		return fmt.Errorf("load schema migrations: %w", err)
	}
	for _, record := range records {
		applied[record.Version] = struct{}{}
	}
	for _, step := range schemaMigrationSteps {
		if _, ok := applied[step.version]; ok {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := step.apply(tx); err != nil {
				return err
			}
			return tx.Create(&SchemaMigration{Version: step.version, Name: step.name, AppliedAt: time.Now().UTC()}).Error
		}); err != nil {
			return fmt.Errorf("schema migration %d (%s) failed: %w", step.version, step.name, err)
		}
	}
	return nil
}

func recoverPendingContextCheckpoints(db *gorm.DB) error {
	return db.Model(&domain.ContextCheckpoint{}).Where("status = ?", "pending").Update("status", "interrupted").Error
}
