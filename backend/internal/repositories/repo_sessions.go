package repositories

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func (r *Repository) ListSessions() ([]models.Session, error) {
	var sessions []models.Session
	err := r.db.Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func (r *Repository) GetSessionByID(id string) (*models.Session, error) {
	var session models.Session
	err := r.db.First(&session, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &session, err
}

func (r *Repository) CreateSession(name string) (*models.Session, error) {
	session := &models.Session{
		ID:   uuid.NewString(),
		Name: name,
	}
	err := r.db.Create(session).Error
	return session, err
}

func (r *Repository) CreateSessionWithID(id, name string) (*models.Session, error) {
	session := &models.Session{
		ID:   id,
		Name: name,
	}
	err := r.db.Create(session).Error
	return session, err
}

func (r *Repository) RenameSessionByUser(id, name string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "is_title_locked": true, "updated_at": time.Now()}).
		Error
}

func (r *Repository) UpdateSessionTitle(id, name string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "updated_at": time.Now()}).
		Error
}

func (r *Repository) DeleteSession(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", id).Delete(&models.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&models.ToolCallRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&models.Session{}).Error
	})
}

func (r *Repository) SetSessionModel(sessionID, modelConfigID string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"model_config_id": modelConfigID, "updated_at": time.Now()}).
		Error
}
