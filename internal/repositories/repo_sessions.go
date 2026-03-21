package repositories

import (
	"errors"
	"slimebot/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListSessions() ([]domain.Session, error) {
	var sessions []domain.Session
	err := r.db.Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func (r *Repository) GetSessionByID(id string) (*domain.Session, error) {
	var session domain.Session
	err := r.db.First(&session, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &session, err
}

func (r *Repository) CreateSession(name string) (*domain.Session, error) {
	session := &domain.Session{
		ID:   uuid.NewString(),
		Name: name,
	}
	err := r.db.Create(session).Error
	return session, err
}

func (r *Repository) CreateSessionWithID(id, name string) (*domain.Session, error) {
	session := &domain.Session{
		ID:   id,
		Name: name,
	}
	err := r.db.Create(session).Error
	return session, err
}

func (r *Repository) RenameSessionByUser(id, name string) error {
	return r.db.Model(&domain.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "is_title_locked": true, "updated_at": time.Now()}).
		Error
}

func (r *Repository) UpdateSessionTitle(id, name string) error {
	return r.db.Model(&domain.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "updated_at": time.Now()}).
		Error
}

func (r *Repository) DeleteSession(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&domain.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.ToolCallRecord{}).Error; err != nil {
			return err
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
