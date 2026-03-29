package repositories

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"slimebot/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

func (r *Repository) UpsertToolCallStart(ctx context.Context, input domain.ToolCallStartRecordInput) error {
	paramsJSONBytes, err := json.Marshal(input.Params)
	if err != nil {
		return err
	}
	paramsJSON := string(paramsJSONBytes)

	startedAt := input.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	record := domain.ToolCallRecord{
		ID:               uuid.NewString(),
		SessionID:        input.SessionID,
		RequestID:        input.RequestID,
		ToolCallID:       input.ToolCallID,
		ToolName:         input.ToolName,
		Command:          input.Command,
		ParamsJSON:       paramsJSON,
		Status:           input.Status,
		RequiresApproval: input.RequiresApproval,
		StartedAt:        startedAt,
	}
	return r.dbWithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "session_id"},
			{Name: "request_id"},
			{Name: "tool_call_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"tool_name":            input.ToolName,
			"command":              input.Command,
			"params_json":          paramsJSON,
			"status":               input.Status,
			"requires_approval":    input.RequiresApproval,
			"started_at":           startedAt,
			"finished_at":          nil,
			"output":               "",
			"error":                "",
			"assistant_message_id": nil,
			"updated_at":           time.Now(),
		}),
	}).Create(&record).Error
}

func (r *Repository) UpdateToolCallResult(ctx context.Context, input domain.ToolCallResultRecordInput) error {
	updates := map[string]any{
		"status":      input.Status,
		"output":      input.Output,
		"error":       input.Error,
		"updated_at":  time.Now(),
		"finished_at": input.FinishedAt,
	}
	if input.FinishedAt.IsZero() {
		updates["finished_at"] = time.Now()
	}

	return r.dbWithContext(ctx).Model(&domain.ToolCallRecord{}).
		Where("session_id = ? AND request_id = ? AND tool_call_id = ?", input.SessionID, input.RequestID, input.ToolCallID).
		Updates(updates).
		Error
}

func (r *Repository) BindToolCallsToAssistantMessage(ctx context.Context, sessionID, requestID, assistantMessageID string) error {
	return r.dbWithContext(ctx).Model(&domain.ToolCallRecord{}).
		Where("session_id = ? AND request_id = ?", sessionID, requestID).
		Updates(map[string]any{
			"assistant_message_id": assistantMessageID,
			"updated_at":           time.Now(),
		}).
		Error
}

func (r *Repository) ListSessionToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error) {
	if len(messageIDs) == 0 {
		return []domain.ToolCallRecord{}, nil
	}
	filtered := make([]string, 0, len(messageIDs))
	for _, id := range messageIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return []domain.ToolCallRecord{}, nil
	}
	var records []domain.ToolCallRecord
	err := r.db.
		Where("session_id = ?", sessionID).
		Where("assistant_message_id IN ?", filtered).
		Order("started_at asc").
		Order("created_at asc").
		Find(&records).
		Error
	return records, err
}
