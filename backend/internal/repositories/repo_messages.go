package repositories

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

func (r *Repository) ListSessionMessages(sessionID string) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages).Error
	return messages, err
}

func (r *Repository) ListSessionMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]models.Message, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	base := r.db.Where("session_id = ?", sessionID)
	var (
		messages []models.Message
		err      error
		hasMore  bool
	)

	switch {
	case after != nil:
		err = base.
			Where("created_at > ?", after.UTC()).
			Order("created_at asc, id asc").
			Limit(limit).
			Find(&messages).
			Error
		if err != nil {
			return nil, false, err
		}
		if len(messages) == 0 {
			return messages, false, nil
		}
		var count int64
		latest := messages[len(messages)-1].CreatedAt
		err = base.Where("created_at > ?", latest).Count(&count).Error
		if err != nil {
			return nil, false, err
		}
		hasMore = count > 0
	default:
		query := base
		if before != nil {
			query = query.Where("created_at < ?", before.UTC())
		}
		err = query.
			Order("created_at desc, id desc").
			Limit(limit).
			Find(&messages).
			Error
		if err != nil {
			return nil, false, err
		}
		if len(messages) == 0 {
			return messages, false, nil
		}
		for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
			messages[left], messages[right] = messages[right], messages[left]
		}
		var count int64
		oldest := messages[0].CreatedAt
		err = base.Where("created_at < ?", oldest).Count(&count).Error
		if err != nil {
			return nil, false, err
		}
		hasMore = count > 0
	}

	return messages, hasMore, nil
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
