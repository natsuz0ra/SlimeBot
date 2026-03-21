package repositories

import (
	"encoding/json"
	"time"

	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func encodeMessageAttachments(items []domain.MessageAttachment) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeMessageAttachments(raw string) []domain.MessageAttachment {
	if raw == "" {
		return []domain.MessageAttachment{}
	}
	var items []domain.MessageAttachment
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []domain.MessageAttachment{}
	}
	return items
}

func normalizeMessages(items []domain.Message) {
	for idx := range items {
		items[idx].Attachments = decodeMessageAttachments(items[idx].AttachmentsJSON)
	}
}

func (r *Repository) ListSessionMessages(sessionID string) ([]domain.Message, error) {
	var messages []domain.Message
	err := r.db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages).Error
	normalizeMessages(messages)
	return messages, err
}

func (r *Repository) ListSessionMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]domain.Message, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	base := r.db.Where("session_id = ?", sessionID)
	var (
		messages []domain.Message
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

	normalizeMessages(messages)
	return messages, hasMore, nil
}

func (r *Repository) ListRecentSessionMessages(sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		return []domain.Message{}, nil
	}

	var messages []domain.Message
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
	normalizeMessages(messages)
	return messages, nil
}

func (r *Repository) AddMessage(sessionID, role, content string) (*domain.Message, error) {
	return r.AddMessageWithInput(domain.AddMessageInput{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	})
}

func (r *Repository) AddMessageWithInput(input domain.AddMessageInput) (*domain.Message, error) {
	message := &domain.Message{
		ID:                uuid.NewString(),
		SessionID:         input.SessionID,
		Role:              input.Role,
		Content:           input.Content,
		IsInterrupted:     input.IsInterrupted,
		IsStopPlaceholder: input.IsStopPlaceholder,
		AttachmentsJSON:   encodeMessageAttachments(input.Attachments),
		Attachments:       input.Attachments,
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&domain.Session{}).
			Where("id = ?", input.SessionID).
			Update("updated_at", time.Now()).
			Error
	})
	return message, err
}
