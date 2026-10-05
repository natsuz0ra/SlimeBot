package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	llmsvc "slimebot/internal/services/llm"
	"strings"
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

func encodeTokenUsage(usage *llmsvc.TokenUsage) string {
	if usage == nil || usage.IsZero() {
		return ""
	}
	data, err := json.Marshal(usage)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeTokenUsage(raw string) *llmsvc.TokenUsage {
	if raw == "" {
		return nil
	}
	var usage llmsvc.TokenUsage
	if err := json.Unmarshal([]byte(raw), &usage); err != nil || usage.IsZero() {
		return nil
	}
	return &usage
}

func normalizeMessages(items []domain.Message) {
	for idx := range items {
		items[idx].Attachments = decodeMessageAttachments(items[idx].AttachmentsJSON)
		items[idx].TokenUsage = decodeTokenUsage(items[idx].TokenUsageJSON)
	}
}

func (r *Repository) ListSessionMessagesPage(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error) {
	if limit <= 0 {
		limit = 10
	}
	fetchLimit := limit + 1

	base := r.dbWithContext(ctx).Where("session_id = ?", sessionID)
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

func (r *Repository) ListAllSessionMessages(ctx context.Context, sessionID string, limit int) ([]domain.Message, error) {
	if limit == 0 {
		limit = 10000
	}
	var messages []domain.Message
	err := r.dbWithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("seq asc").
		Limit(limit).
		Find(&messages).
		Error
	if err != nil {
		return nil, err
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
		TokenUsageJSON:    encodeTokenUsage(input.TokenUsage),
		Attachments:       input.Attachments,
		TokenUsage:        input.TokenUsage,
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
		if err := bumpContextHistory(tx, input.SessionID); err != nil {
			return err
		}
		return tx.Model(&domain.Session{}).
			Where("id = ?", input.SessionID).
			Update("updated_at", time.Now()).
			Error
	})
	return message, err
}

func (r *Repository) UpdateUserMessageAndPruneAfter(ctx context.Context, sessionID, messageID, content string) (*domain.Message, error) {
	trimmedSessionID := strings.TrimSpace(sessionID)
	trimmedMessageID := strings.TrimSpace(messageID)
	if trimmedSessionID == "" || trimmedMessageID == "" {
		return nil, fmt.Errorf("message edit: %w", apperrors.ErrInvalidInput)
	}
	var updated domain.Message
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target domain.Message
		if err := tx.
			Where("session_id = ? AND id = ?", trimmedSessionID, trimmedMessageID).
			Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("message %s: %w", trimmedMessageID, apperrors.ErrNotFound)
			}
			return err
		}
		if target.Role != "user" {
			return fmt.Errorf("message %s is not editable: %w", trimmedMessageID, apperrors.ErrInvalidInput)
		}

		var latestUser domain.Message
		if err := tx.
			Where("session_id = ? AND role = ?", trimmedSessionID, "user").
			Order("seq desc").
			Limit(1).
			Take(&latestUser).Error; err != nil {
			return err
		}
		if latestUser.ID != target.ID {
			return fmt.Errorf("message %s is not the latest user message: %w", trimmedMessageID, apperrors.ErrInvalidInput)
		}

		var prunedAssistantIDs []string
		if err := tx.Model(&domain.Message{}).
			Where("session_id = ? AND role = ? AND seq > ?", trimmedSessionID, "assistant", target.Seq).
			Pluck("id", &prunedAssistantIDs).Error; err != nil {
			return err
		}
		if len(prunedAssistantIDs) > 0 {
			var teamRunIDs []string
			if err := tx.Model(&domain.TeamRun{}).
				Where("session_id = ? AND assistant_message_id IN ?", trimmedSessionID, prunedAssistantIDs).
				Pluck("id", &teamRunIDs).Error; err != nil {
				return err
			}
			if len(teamRunIDs) > 0 {
				if err := tx.Where("team_run_id IN ?", teamRunIDs).Delete(&domain.TeamMemberRun{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("session_id = ? AND assistant_message_id IN ?", trimmedSessionID, prunedAssistantIDs).
				Delete(&domain.TeamRun{}).Error; err != nil {
				return err
			}
			if err := tx.Where("session_id = ? AND assistant_message_id IN ?", trimmedSessionID, prunedAssistantIDs).
				Delete(&domain.ToolCallRecord{}).Error; err != nil {
				return err
			}
			if err := tx.Where("session_id = ? AND assistant_message_id IN ?", trimmedSessionID, prunedAssistantIDs).
				Delete(&domain.ThinkingRecord{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("session_id = ? AND seq > ?", trimmedSessionID, target.Seq).
			Delete(&domain.Message{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", trimmedSessionID).
			Delete(&domain.SessionContextSummary{}).Error; err != nil {
			return err
		}

		if err := invalidateContext(tx, trimmedSessionID, target.Seq); err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&domain.Message{}).
			Where("id = ?", target.ID).
			Updates(map[string]any{
				"content":    content,
				"created_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.Session{}).
			Where("id = ?", trimmedSessionID).
			Update("updated_at", now).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", target.ID).Take(&updated).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	items := []domain.Message{updated}
	normalizeMessages(items)
	return &items[0], nil
}
