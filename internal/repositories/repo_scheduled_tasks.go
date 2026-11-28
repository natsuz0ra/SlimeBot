package repositories

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) CreateScheduledTask(ctx context.Context, task *domain.ScheduledTask) error {
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	return r.dbWithContext(ctx).Create(task).Error
}

func (r *Repository) GetScheduledTask(ctx context.Context, id string) (*domain.ScheduledTask, error) {
	var task domain.ScheduledTask
	err := r.dbWithContext(ctx).First(&task, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("scheduled task %s: %w", id, apperrors.ErrNotFound)
	}
	return &task, err
}

func (r *Repository) ListScheduledTasks(ctx context.Context, includeInactive bool) ([]domain.ScheduledTask, error) {
	var tasks []domain.ScheduledTask
	q := r.dbWithContext(ctx).Order("created_at desc")
	if !includeInactive {
		q = q.Where("status NOT IN ?", []string{domain.ScheduledTaskStatusPaused, domain.ScheduledTaskStatusCompleted})
	}
	return tasks, q.Find(&tasks).Error
}

func (r *Repository) ListDueScheduledTasks(ctx context.Context, now any) ([]domain.ScheduledTask, error) {
	var tasks []domain.ScheduledTask
	// Compare actual instants regardless of the offsets stored by older schedules.
	err := r.dbWithContext(ctx).
		Where("status = ? AND next_run_at IS NOT NULL AND julianday(next_run_at) <= julianday(?)", domain.ScheduledTaskStatusScheduled, now).
		Order("julianday(next_run_at) asc").
		Find(&tasks).Error
	return tasks, err
}

func (r *Repository) UpdateScheduledTask(ctx context.Context, id string, updates map[string]any) error {
	return r.dbWithContext(ctx).Model(&domain.ScheduledTask{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) DeleteScheduledTask(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&domain.ScheduledTaskRun{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&domain.ScheduledTask{}).Error
	})
}

func (r *Repository) CreateScheduledTaskRun(ctx context.Context, run *domain.ScheduledTaskRun) error {
	return r.dbWithContext(ctx).Create(run).Error
}

// ClaimScheduledTaskRun atomically changes a due task to running and creates
// its execution record. A competing claim or a stale due snapshot is a no-op.
func (r *Repository) ClaimScheduledTaskRun(ctx context.Context, taskID string, claimedAt time.Time, nextRunAt *time.Time, run *domain.ScheduledTaskRun) (bool, error) {
	claimed := false
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&domain.ScheduledTask{}).
			Where("id = ? AND status = ? AND next_run_at IS NOT NULL AND julianday(next_run_at) <= julianday(?)", taskID, domain.ScheduledTaskStatusScheduled, claimedAt).
			Updates(map[string]any{"status": domain.ScheduledTaskStatusRunning, "next_run_at": nextRunAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		run.TaskID = taskID
		run.Status = domain.ScheduledTaskRunStatusRunning
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		claimed = true
		return nil
	})
	return claimed, err
}

// CompleteScheduledTaskRun persists a terminal run and its task counters in
// one transaction. Completing an already terminal run is idempotent.
func (r *Repository) CompleteScheduledTaskRun(ctx context.Context, taskID, runID string, run *domain.ScheduledTaskRun, taskUpdates map[string]any) (bool, error) {
	completed := false
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if runID == "" {
			run.TaskID = taskID
			if err := tx.Create(run).Error; err != nil {
				return err
			}
		} else {
			result := tx.Model(&domain.ScheduledTaskRun{}).
				Where("id = ? AND task_id = ? AND status = ?", runID, taskID, domain.ScheduledTaskRunStatusRunning).
				Updates(map[string]any{
					"session_id": run.SessionID, "request_id": run.RequestID,
					"status": run.Status, "answer": run.Answer, "error": run.Error,
					"started_at": run.StartedAt, "finished_at": run.FinishedAt,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				var existing domain.ScheduledTaskRun
				if err := tx.Select("id").Where("id = ? AND task_id = ?", runID, taskID).Take(&existing).Error; err != nil {
					return err
				}
				return nil
			}
		}
		if result := tx.Model(&domain.ScheduledTask{}).Where("id = ?", taskID).Updates(taskUpdates); result.Error != nil {
			return result.Error
		} else if result.RowsAffected != 1 {
			return fmt.Errorf("scheduled task %s: %w", taskID, apperrors.ErrNotFound)
		}
		completed = true
		return nil
	})
	return completed, err
}

func (r *Repository) ListScheduledTaskRuns(ctx context.Context, taskID, status string, limit, offset int) ([]domain.ScheduledTaskRun, error) {
	runs := []domain.ScheduledTaskRun{}
	q := r.dbWithContext(ctx).Model(&domain.ScheduledTaskRun{}).
		Select("scheduled_task_runs.id, scheduled_task_runs.task_id, scheduled_tasks.name AS task_name, scheduled_task_runs.session_id, scheduled_task_runs.request_id, scheduled_task_runs.status, substr(scheduled_task_runs.answer, 1, 240) AS answer, substr(scheduled_task_runs.error, 1, 240) AS error, scheduled_task_runs.started_at, scheduled_task_runs.finished_at, scheduled_task_runs.created_at, scheduled_task_runs.updated_at").
		Joins("JOIN scheduled_tasks ON scheduled_tasks.id = scheduled_task_runs.task_id")
	if taskID != "" {
		q = q.Where("scheduled_task_runs.task_id = ?", taskID)
	}
	if status != "" {
		q = q.Where("scheduled_task_runs.status = ?", status)
	}
	err := q.Order("scheduled_task_runs.started_at DESC, scheduled_task_runs.id DESC").Limit(limit).Offset(offset).Find(&runs).Error
	return runs, err
}

func (r *Repository) GetScheduledTaskRun(ctx context.Context, id string) (*domain.ScheduledTaskRun, error) {
	var run domain.ScheduledTaskRun
	err := r.dbWithContext(ctx).Model(&domain.ScheduledTaskRun{}).
		Select("scheduled_task_runs.*, scheduled_tasks.name AS task_name").
		Joins("JOIN scheduled_tasks ON scheduled_tasks.id = scheduled_task_runs.task_id").
		Where("scheduled_task_runs.id = ?", id).First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("scheduled task run %s: %w", id, apperrors.ErrNotFound)
	}
	return &run, err
}

func (r *Repository) UpdateScheduledTaskRun(ctx context.Context, id string, updates map[string]any) error {
	return r.dbWithContext(ctx).Model(&domain.ScheduledTaskRun{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) InterruptScheduledTaskRuns(ctx context.Context, now time.Time) error {
	return r.dbWithContext(ctx).Model(&domain.ScheduledTaskRun{}).
		Where("status = ?", domain.ScheduledTaskRunStatusRunning).
		Updates(map[string]any{"status": domain.ScheduledTaskRunStatusInterrupted, "finished_at": now, "error": "Execution was interrupted by an application restart."}).Error
}

func migrateScheduledRunContexts(db *gorm.DB) error {
	return db.Exec(`UPDATE sessions SET kind = ?, is_title_locked = 1
 WHERE kind = ? AND id IN (
  SELECT r.session_id FROM scheduled_task_runs r JOIN scheduled_tasks t ON t.id = r.task_id
  WHERE r.session_id <> '' AND r.session_id <> t.session_id
 ) AND NOT EXISTS (SELECT 1 FROM scheduled_tasks t WHERE t.session_id = sessions.id)`, domain.SessionKindTaskRun, domain.SessionKindChat).Error
}
