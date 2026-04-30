package repositories

import (
	"context"
	"strings"
	"time"

	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	thinkingStatusStreaming = "streaming"
	thinkingStatusCompleted = "completed"
)

func (r *Repository) UpsertThinkingStart(ctx context.Context, input domain.ThinkingStartRecordInput) error {
	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	record := domain.ThinkingRecord{
		ID:               uuid.NewString(),
		SessionID:        input.SessionID,
		RequestID:        input.RequestID,
		ThinkingID:       strings.TrimSpace(input.ThinkingID),
		ParentToolCallID: strings.TrimSpace(input.ParentToolCallID),
		SubagentRunID:    strings.TrimSpace(input.SubagentRunID),
		Status:           thinkingStatusStreaming,
		StartedAt:        startedAt,
	}
	return r.dbWithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "session_id"},
			{Name: "request_id"},
			{Name: "thinking_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"content":              "",
			"status":               thinkingStatusStreaming,
			"started_at":           startedAt,
			"finished_at":          nil,
			"duration_ms":          0,
			"assistant_message_id": nil,
			"parent_tool_call_id":  strings.TrimSpace(input.ParentToolCallID),
			"subagent_run_id":      strings.TrimSpace(input.SubagentRunID),
			"updated_at":           time.Now(),
		}),
	}).Create(&record).Error
}

func (r *Repository) AppendThinkingChunk(ctx context.Context, input domain.ThinkingChunkRecordInput) error {
	if input.Chunk == "" {
		return nil
	}
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record domain.ThinkingRecord
		if err := tx.
			Where("session_id = ? AND request_id = ? AND thinking_id = ?", input.SessionID, input.RequestID, input.ThinkingID).
			Take(&record).Error; err != nil {
			return err
		}
		return tx.Model(&domain.ThinkingRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]any{
				"content":    record.Content + input.Chunk,
				"updated_at": time.Now(),
			}).Error
	})
}

func (r *Repository) FinishThinking(ctx context.Context, input domain.ThinkingFinishRecordInput) error {
	finishedAt := input.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record domain.ThinkingRecord
		if err := tx.
			Where("session_id = ? AND request_id = ? AND thinking_id = ?", input.SessionID, input.RequestID, input.ThinkingID).
			Take(&record).Error; err != nil {
			return err
		}
		durationMs := finishedAt.Sub(record.StartedAt).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
		return tx.Model(&domain.ThinkingRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]any{
				"status":      thinkingStatusCompleted,
				"finished_at": finishedAt,
				"duration_ms": durationMs,
				"updated_at":  time.Now(),
			}).Error
	})
}

func (r *Repository) FinishOpenThinkingForRequest(ctx context.Context, sessionID, requestID string) error {
	finishedAt := time.Now()
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []domain.ThinkingRecord
		if err := tx.
			Where("session_id = ? AND request_id = ? AND status = ?", sessionID, requestID, thinkingStatusStreaming).
			Find(&records).Error; err != nil {
			return err
		}
		for _, record := range records {
			durationMs := finishedAt.Sub(record.StartedAt).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
			if err := tx.Model(&domain.ThinkingRecord{}).
				Where("id = ?", record.ID).
				Updates(map[string]any{
					"status":      thinkingStatusCompleted,
					"finished_at": finishedAt,
					"duration_ms": durationMs,
					"updated_at":  finishedAt,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) BindThinkingRecordsToAssistantMessage(ctx context.Context, sessionID, requestID, assistantMessageID string) error {
	return r.dbWithContext(ctx).Model(&domain.ThinkingRecord{}).
		Where("session_id = ? AND request_id = ?", sessionID, requestID).
		Updates(map[string]any{
			"assistant_message_id": assistantMessageID,
			"updated_at":           time.Now(),
		}).
		Error
}

func (r *Repository) ListSessionThinkingRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.ThinkingRecord, error) {
	if len(messageIDs) == 0 {
		return []domain.ThinkingRecord{}, nil
	}
	filtered := make([]string, 0, len(messageIDs))
	for _, id := range messageIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return []domain.ThinkingRecord{}, nil
	}
	var records []domain.ThinkingRecord
	err := r.dbWithContext(ctx).
		Where("session_id = ?", sessionID).
		Where("assistant_message_id IN ?", filtered).
		Order("started_at asc").
		Order("created_at asc").
		Find(&records).
		Error
	return records, err
}
