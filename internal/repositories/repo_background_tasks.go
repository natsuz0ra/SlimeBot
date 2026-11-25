package repositories

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
)

func (r *Repository) SaveBackgroundTask(ctx context.Context, task *domain.BackgroundTask) error {
	return r.dbWithContext(ctx).Save(task).Error
}

func (r *Repository) GetBackgroundTask(ctx context.Context, id string) (*domain.BackgroundTask, error) {
	var task domain.BackgroundTask
	if err := r.dbWithContext(ctx).First(&task, "id = ?", id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) ListBackgroundTasks(ctx context.Context, sessionID string, limit int) ([]domain.BackgroundTask, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	items := []domain.BackgroundTask{}
	query := r.dbWithContext(ctx).Order("created_at DESC").Limit(limit)
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}
	return items, query.Find(&items).Error
}

func (r *Repository) InterruptBackgroundTasks(ctx context.Context, now time.Time) error {
	return r.dbWithContext(ctx).Model(&domain.BackgroundTask{}).
		Where("status IN ?", []string{domain.BackgroundStatusRunning, domain.BackgroundStatusQueued, domain.BackgroundStatusWaking}).
		Updates(map[string]any{"status": domain.BackgroundStatusInterrupted, "error": "SlimeBot 重启，原命令未重新执行。可手动重试或使用已保存的结果继续。", "finished_at": now, "updated_at": now}).Error
}
