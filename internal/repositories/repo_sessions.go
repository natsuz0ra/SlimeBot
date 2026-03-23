package repositories

import (
	"context"
	"errors"
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

func (r *Repository) ListSessions(limit int, offset int, query string) ([]domain.Session, error) {
	var sessions []domain.Session
	q := r.db.Order("updated_at desc")
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
		return nil, nil
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

func (r *Repository) RenameSessionByUser(id, name string) error {
	return r.db.Model(&domain.Session{}).
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

func (r *Repository) DeleteSession(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&domain.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.ToolCallRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.SessionMemory{}).Error; err != nil {
			return err
		}
		var ftscount int64
		if err := tx.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='session_memories_fts'`).Scan(&ftscount).Error; err == nil && ftscount > 0 {
			_ = tx.Exec(`DELETE FROM session_memories_fts WHERE session_id = ?`, id).Error
		}
		return tx.Where("id = ?", id).Delete(&domain.Session{}).Error
	})
}

func (r *Repository) SetSessionModel(sessionID, modelConfigID string) error {
	return r.db.Model(&domain.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"model_config_id": modelConfigID, "updated_at": time.Now()}).
		Error
}
