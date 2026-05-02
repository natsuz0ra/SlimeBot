package repositories

import (
	"context"
	"time"

	"slimebot/internal/domain"

	"gorm.io/gorm"
)

func (r *Repository) EnqueueMemoryWriteJob(ctx context.Context, job *domain.MemoryWriteJob) error {
	return r.dbWithContext(ctx).Create(job).Error
}

func (r *Repository) ClaimPendingMemoryWriteJobs(ctx context.Context, limit int) ([]domain.MemoryWriteJob, error) {
	if limit <= 0 {
		limit = 1
	}
	now := time.Now()
	var jobs []domain.MemoryWriteJob
	if err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("status = ? AND next_retry_at <= ?", "pending", now).
			Order("next_retry_at asc, created_at asc").
			Limit(limit).
			Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		claimedAt := time.Now()
		for idx := range jobs {
			if err := tx.Model(&domain.MemoryWriteJob{}).
				Where("id = ? AND status = ?", jobs[idx].ID, "pending").
				Updates(map[string]any{
					"status":     "processing",
					"claimed_at": claimedAt,
					"updated_at": claimedAt,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	claimed := make([]domain.MemoryWriteJob, 0, len(jobs))
	for _, item := range jobs {
		var current domain.MemoryWriteJob
		if err := r.dbWithContext(ctx).
			Where("id = ? AND status = ?", item.ID, "processing").
			Take(&current).Error; err == nil {
			claimed = append(claimed, current)
		}
	}
	return claimed, nil
}

func (r *Repository) MarkMemoryWriteJobDone(ctx context.Context, jobID string) error {
	now := time.Now()
	return r.dbWithContext(ctx).Model(&domain.MemoryWriteJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":      "done",
			"last_error":  "",
			"finished_at": now,
			"updated_at":  now,
		}).Error
}

func (r *Repository) MarkMemoryWriteJobRetry(ctx context.Context, jobID string, nextRetryAt time.Time, errText string) error {
	now := time.Now()
	return r.dbWithContext(ctx).Model(&domain.MemoryWriteJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":        "pending",
			"attempt":       gorm.Expr("attempt + 1"),
			"next_retry_at": nextRetryAt,
			"last_error":    errText,
			"updated_at":    now,
		}).Error
}

func (r *Repository) MarkMemoryWriteJobDead(ctx context.Context, jobID string, errText string) error {
	now := time.Now()
	return r.dbWithContext(ctx).Model(&domain.MemoryWriteJob{}).
		Where("id = ?", jobID).
		Updates(map[string]any{
			"status":      "dead",
			"last_error":  errText,
			"finished_at": now,
			"updated_at":  now,
		}).Error
}
