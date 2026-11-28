package repositories

import (
	"context"
	"errors"
	"strings"

	"slimebot/internal/domain"

	"gorm.io/gorm"
)

func (r *Repository) CompleteChatTurn(ctx context.Context, input domain.CompleteChatTurnInput) (*domain.CompleteChatTurnResult, error) {
	result := &domain.CompleteChatTurnResult{}
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.RequestID != "" {
			var existing domain.Message
			err := tx.Where("session_id = ? AND role = 'assistant' AND request_id = ?", input.SessionID, input.RequestID).
				Take(&existing).Error
			if err == nil {
				existing.Attachments = decodeMessageAttachments(existing.AttachmentsJSON)
				existing.TokenUsage = decodeTokenUsage(existing.TokenUsageJSON)
				result.AssistantMessage = &existing
				result.Deduplicated = true
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		store := New(tx)
		if input.Interrupted {
			if err := store.FinishOpenToolCallsForRequest(ctx, input.SessionID, input.RequestID, "Execution cancelled."); err != nil {
				return err
			}
			if err := finishOpenThinking(tx, input.SessionID, input.RequestID); err != nil {
				return err
			}
		}
		assistant, err := addMessage(tx, domain.AddMessageInput{
			SessionID:         input.SessionID,
			RequestID:         input.RequestID,
			Role:              "assistant",
			Content:           input.Content,
			IsInterrupted:     input.Interrupted,
			IsStopPlaceholder: input.Interrupted && strings.TrimSpace(input.Content) == "",
			TokenUsage:        input.TokenUsage,
		})
		if err != nil {
			return err
		}
		if err := store.BindToolCallsToAssistantMessage(ctx, input.SessionID, input.RequestID, assistant.ID); err != nil {
			return err
		}
		if err := store.BindThinkingRecordsToAssistantMessage(ctx, input.SessionID, input.RequestID, assistant.ID); err != nil {
			return err
		}
		if input.ContextRequestID != "" {
			if err := store.BindContextRequest(ctx, input.SessionID, input.ContextRequestID, assistant.ID); err != nil {
				return err
			}
		}
		result.AssistantMessage = assistant
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
