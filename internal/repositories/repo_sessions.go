package repositories

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func escapeSQLiteLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

func (r *Repository) ListSessions(ctx context.Context, limit int, offset int, query string) ([]domain.Session, error) {
	var sessions []domain.Session
	q := r.dbWithContext(ctx).Order("updated_at desc")
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		like := "%" + escapeSQLiteLikePattern(trimmed) + "%"
		q = q.Where("name LIKE ? ESCAPE '\\'", like)
	}
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	err := q.Find(&sessions).Error
	return sessions, err
}

func (r *Repository) GetSessionByID(ctx context.Context, id string) (*domain.Session, error) {
	var session domain.Session
	err := r.dbWithContext(ctx).First(&session, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("session %s: %w", id, apperrors.ErrNotFound)
	}
	return &session, err
}

func (r *Repository) CreateSession(ctx context.Context, name string) (*domain.Session, error) {
	session := &domain.Session{
		ID:   uuid.NewString(),
		Name: name,
	}
	err := r.dbWithContext(ctx).Create(session).Error
	return session, err
}

func (r *Repository) CreateSessionWithID(ctx context.Context, id, name string) (*domain.Session, error) {
	session := &domain.Session{
		ID:   id,
		Name: name,
	}
	err := r.dbWithContext(ctx).Create(session).Error
	return session, err
}

func (r *Repository) RenameSessionByUser(ctx context.Context, id, name string) error {
	return r.dbWithContext(ctx).Model(&domain.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "is_title_locked": true, "updated_at": time.Now()}).
		Error
}

func (r *Repository) UpdateSessionTitle(ctx context.Context, id, name string) (bool, error) {
	result := r.dbWithContext(ctx).Model(&domain.Session{}).
		Where("id = ? AND is_title_locked = ? AND name <> ?", id, false, name).
		Updates(map[string]any{"name": name, "updated_at": time.Now()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) DeleteSession(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete messages.
		if err := tx.Table("messages").Where("session_id = ?", id).Delete(nil).Error; err != nil {
			return err
		}
		// Delete tool call records.
		if err := tx.Table("tool_call_records").Where("session_id = ?", id).Delete(nil).Error; err != nil {
			return err
		}
		// Delete the session row.
		return tx.Table("sessions").Where("id = ?", id).Delete(nil).Error
	})
}
