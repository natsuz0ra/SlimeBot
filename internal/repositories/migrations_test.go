package repositories

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"slimebot/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRunMigrationsFreshDatabaseAndIdempotency(t *testing.T) {
	db := NewSQLiteDBTest(t, "migrations_fresh")
	for _, model := range append(AllModels(), &SchemaMigration{}) {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("table for %T was not created", model)
		}
	}
	var first []SchemaMigration
	if err := db.Order("version asc").Find(&first).Error; err != nil {
		t.Fatal(err)
	}
	if len(first) != len(schemaMigrationSteps) {
		t.Fatalf("applied migrations = %+v", first)
	}
	for i, record := range first {
		if record.Version != i+1 || record.Name == "" || record.AppliedAt.IsZero() {
			t.Fatalf("invalid migration record: %+v", record)
		}
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	var second []SchemaMigration
	if err := db.Order("version asc").Find(&second).Error; err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("duplicate migration records: %+v", second)
	}
	for i := range first {
		if second[i].Version != first[i].Version || !second[i].AppliedAt.Equal(first[i].AppliedAt) {
			t.Fatalf("migration %d was reapplied: first=%+v second=%+v", i, first[i], second[i])
		}
	}
}

func TestRunMigrationsUpgradesLegacyData(t *testing.T) {
	db := openMigrationTestDB(t)
	legacyModels := []any{&domain.Session{}, &domain.LLMConfig{}, &domain.ScheduledTask{}, &domain.ScheduledTaskRun{}}
	if err := db.AutoMigrate(legacyModels...); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := New(db)
	if _, err := repo.CreateSessionWithID(ctx, "source", "Source"); err != nil {
		t.Fatal(err)
	}
	execution := &domain.Session{ID: "execution", Kind: domain.SessionKindChat, Name: "Legacy run", IsTitleLocked: false}
	if err := db.Create(execution).Error; err != nil {
		t.Fatal(err)
	}
	for _, model := range []domain.LLMConfig{
		{ID: "model-1", Name: "one", Provider: "openai", BaseURL: "https://legacy.example/v1", APIKey: "secret", Model: "gpt"},
		{ID: "model-2", Name: "two", Provider: "openai", BaseURL: "https://legacy.example/v1", APIKey: "secret", Model: "gpt-mini"},
	} {
		if err := db.Create(&model).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	if err := db.Create(&domain.ScheduledTask{
		ID: "legacy-task", Name: "legacy", Prompt: "run", SessionID: "source",
		ScheduleKind: string(domain.ScheduleKindOnce), RunAt: &now, Timezone: "UTC", Status: domain.ScheduledTaskStatusCompleted,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScheduledTaskRun{
		ID: "legacy-run", TaskID: "legacy-task", SessionID: "execution", RequestID: "request",
		Status: domain.ScheduledTaskRunStatusOK, StartedAt: now, FinishedAt: &now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	models, err := repo.ListLLMConfigs(ctx)
	if err != nil || len(models) != 2 || models[0].ProviderID == "" || models[0].ProviderID != models[1].ProviderID {
		t.Fatalf("migrated models = %+v, %v", models, err)
	}
	providers, err := repo.ListLLMProviders(ctx)
	if err != nil || len(providers) != 1 || providers[0].BaseURL != "https://legacy.example/v1" {
		t.Fatalf("migrated providers = %+v, %v", providers, err)
	}
	var source, runSession domain.Session
	if err := db.Where("id = ?", "source").Take(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("id = ?", "execution").Take(&runSession).Error; err != nil {
		t.Fatal(err)
	}
	if source.Kind != domain.SessionKindChat || runSession.Kind != domain.SessionKindTaskRun || !runSession.IsTitleLocked {
		t.Fatalf("legacy sessions not upgraded: source=%+v execution=%+v", source, runSession)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	providers, err = repo.ListLLMProviders(ctx)
	if err != nil || len(providers) != 1 {
		t.Fatalf("providers after idempotent upgrade = %+v, %v", providers, err)
	}
}

func TestNewSQLiteRecoversPendingCheckpointsOnEveryRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := &domain.ContextCheckpoint{ID: "pending-after-first-start", ScopeID: "session", SourceIDs: "[]", InputHash: "hash", Status: "pending"}
	if err := db.Create(checkpoint).Error; err != nil {
		t.Fatal(err)
	}
	if err := New(db).Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = New(restarted).Close() }()
	var recovered domain.ContextCheckpoint
	if err := restarted.Where("id = ?", checkpoint.ID).Take(&recovered).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "interrupted" {
		t.Fatalf("pending checkpoint after restart = %q", recovered.Status)
	}
	var migrations int64
	if err := restarted.Model(&SchemaMigration{}).Count(&migrations).Error; err != nil || migrations != int64(len(schemaMigrationSteps)) {
		t.Fatalf("migration records after restart = %d, %v", migrations, err)
	}
}

func openMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: newGormSlogLogger(200 * time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	return db
}
