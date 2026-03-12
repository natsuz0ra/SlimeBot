package repositories

import (
	"errors"
	"time"

	"corner/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
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
	tx := r.db.Begin()
	if err := tx.Where("session_id = ?", id).Delete(&models.Message{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("id = ?", id).Delete(&models.Session{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (r *Repository) SetSessionModel(sessionID, modelConfigID string) error {
	return r.db.Model(&models.Session{}).
		Where("id = ?", sessionID).
		Updates(map[string]any{"model_config_id": modelConfigID, "updated_at": time.Now()}).
		Error
}
