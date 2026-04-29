package repositories

import (
	"context"
	"encoding/json"
	"errors"
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

func (r *Repository) ListSessionMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error) {
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit + 1

	base := r.db.Where("session_id = ?", sessionID)
	var messages []domain.Message
	var hasMore bool

	switch {
	case after != nil:
		if err := base.Where("(created_at > ?) OR (created_at = ? AND seq > ?)", *after, *after, *afterSeq).
			Order("created_at asc, seq asc").
			Limit(fetchLimit).
			Find(&messages).Error; err != nil {
			return nil, false, err
		}
		messages, hasMore = FetchWindow(messages, limit)
	default:
		query := base
		if before != nil {
			query = query.Where("(created_at < ?) OR (created_at = ? AND seq < ?)", *before, *before, *beforeSeq)
		}
		if err := query.
			Order("created_at desc, seq desc").
			Limit(fetchLimit).
			Find(&messages).Error; err != nil {
			return nil, false, err
		}
		// Trim oldest from the tail of the newest-first list, then reverse to chronological order.
		messages, hasMore = FetchWindow(messages, limit)
		for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
			messages[left], messages[right] = messages[right], messages[left]
		}
	}

	if len(messages) == 0 {
		return messages, false, nil
	}
	normalizeMessages(messages)
	return messages, hasMore, nil
}

func (r *Repository) ListRecentSessionMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		return []domain.Message{}, nil
	}

	var messages []domain.Message
	err := r.dbWithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at desc, seq desc").
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

func (r *Repository) AddMessageWithInput(ctx context.Context, input domain.AddMessageInput) (*domain.Message, error) {
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
	if !input.CreatedAt.IsZero() {
		message.CreatedAt = input.CreatedAt
	}
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var last domain.Message
		if err := tx.Model(&domain.Message{}).
			Select("seq").
			Where("session_id = ?", input.SessionID).
			Order("seq desc").
			Limit(1).
			Take(&last).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		message.Seq = last.Seq + 1
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
