package repositories

import (
	"time"

	"corner/backend/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (r *Repository) ListSessionMessages(sessionID string) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages).Error
	return messages, err
}

func (r *Repository) ListRecentSessionMessages(sessionID string, limit int) ([]models.Message, error) {
	if limit <= 0 {
		return []models.Message{}, nil
	}

	var messages []models.Message
	err := r.db.
		Where("session_id = ?", sessionID).
		Order("created_at desc").
		Limit(limit).
		Find(&messages).
		Error
	if err != nil {
		return nil, err
	}

	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, nil
}

func (r *Repository) AddMessage(sessionID, role, content string) (*models.Message, error) {
	message := &models.Message{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&models.Session{}).
			Where("id = ?", sessionID).
			Update("updated_at", time.Now()).
			Error
	})
	return message, err
}
