package repositories

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
	"slimebot/internal/domain"
)

func TestCheckpointTransactionRollsBackHeadAndReplacement(t *testing.T) {
	db := NewSQLiteDBTest(t, "checkpoint_rollback")
	repo := New(db)
	ctx := context.Background()
	head := domain.ContextHead{ScopeID: "scope", HistoryRevision: 7, ProjectionRevision: 2}
	old := domain.ContextCheckpoint{ID: "old", ScopeID: "scope", SourceIDs: "[1]", Status: "committed", Active: true}
	newCP := domain.ContextCheckpoint{ID: "new", ScopeID: "scope", SourceIDs: "[1,2]", Status: "pending", HistoryRevision: 7, ProjectionRevision: 2, Summary: "validated summary"}
	for _, value := range []any{&head, &old, &newCP} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("simulated checkpoint disk write failure")
	if err := db.Callback().Update().Before("gorm:update").Register("test:checkpoint_write_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "context_checkpoints" {
			if values, ok := tx.Statement.Dest.(map[string]any); ok && values["status"] == "committed" {
				tx.AddError(failure)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitContextCheckpoint(ctx, &newCP, []string{old.ID}); !errors.Is(err, failure) {
		t.Fatalf("commit error lost: %v", err)
	}
	snapshot, err := repo.GetContextSnapshot(ctx, "scope")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Head.ProjectionRevision != 2 || snapshot.Head.HistoryRevision != 7 || len(snapshot.Checkpoints) != 1 || snapshot.Checkpoints[0].ID != old.ID {
		t.Fatalf("failed commit changed active projection: %+v", snapshot)
	}
	if err := repo.FinishContextAttempt(ctx, newCP.ID, "failed", "storage failure"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteSession(ctx, "scope"); err != nil {
		t.Fatal(err)
	}
	for _, model := range []any{&domain.ContextHead{}, &domain.ContextCheckpoint{}, &domain.ContextEntry{}} {
		var count int64
		if err := db.Model(model).Where("scope_id = ?", "scope").Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("session delete left context state: %T count=%d err=%v", model, count, err)
		}
	}
}
