package repositories

import (
	"context"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestMemoryWriteJobLifecycle(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "memory_jobs"))
	ctx := context.Background()
	job := &domain.MemoryWriteJob{
		ID:                 "job-1",
		SessionID:          "s1",
		AssistantMessageID: "a1",
		MessageContent:     "assistant content",
		HistoryDigest:      "digest",
		Status:             "pending",
		NextRetryAt:        time.Now().Add(-time.Second),
	}
	if err := repo.EnqueueMemoryWriteJob(ctx, job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	claimed, err := repo.ClaimPendingMemoryWriteJobs(ctx, 1)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}
	if claimed[0].Status != "processing" {
		t.Fatalf("expected processing status, got %s", claimed[0].Status)
	}

	if err := repo.MarkMemoryWriteJobRetry(ctx, job.ID, time.Now().Add(time.Minute), "temporary"); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	var got domain.MemoryWriteJob
	if err := repo.dbWithContext(ctx).Where("id = ?", job.ID).Take(&got).Error; err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got.Status != "pending" || got.Attempt != 1 {
		t.Fatalf("unexpected retry state: status=%s attempt=%d", got.Status, got.Attempt)
	}

	if err := repo.MarkMemoryWriteJobDone(ctx, job.ID); err != nil {
		t.Fatalf("done failed: %v", err)
	}
	if err := repo.dbWithContext(ctx).Where("id = ?", job.ID).Take(&got).Error; err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if got.Status != "done" || got.FinishedAt == nil {
		t.Fatalf("unexpected done state: status=%s finished_at=%v", got.Status, got.FinishedAt)
	}
}
