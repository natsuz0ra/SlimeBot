package repositories

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"slimebot/internal/constants"
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
		ModelFuncName:    strings.TrimSpace(input.ModelFuncName),
		ParamsJSON:       paramsJSON,
		Status:           input.Status,
		RequiresApproval: input.RequiresApproval,
		ParentToolCallID: strings.TrimSpace(input.ParentToolCallID),
		SubagentRunID:    strings.TrimSpace(input.SubagentRunID),
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
			"model_func_name":      strings.TrimSpace(input.ModelFuncName),
			"params_json":          paramsJSON,
			"status":               input.Status,
			"requires_approval":    input.RequiresApproval,
			"parent_tool_call_id":  record.ParentToolCallID,
			"subagent_run_id":      record.SubagentRunID,
			"started_at":           startedAt,
			"finished_at":          nil,
			"output":               "",
			"error":                "",
			"metadata_json":        "",
			"assistant_message_id": nil,
			"updated_at":           time.Now(),
		}),
	}).Create(&record).Error
}

func (r *Repository) FinishOpenToolCallsForRequest(ctx context.Context, sessionID, requestID, errorMessage string) error {
	trimmedError := strings.TrimSpace(errorMessage)
	if trimmedError == "" {
		trimmedError = "Execution cancelled."
	}
	now := time.Now()
	return r.dbWithContext(ctx).Model(&domain.ToolCallRecord{}).
		Where("session_id = ? AND request_id = ?", sessionID, requestID).
		Where("status IN ?", []string{constants.ToolCallStatusPending, constants.ToolCallStatusReviewing, constants.ToolCallStatusExecuting}).
		Updates(map[string]any{
			"status":      constants.ToolCallStatusError,
			"error":       trimmedError,
			"finished_at": now,
			"updated_at":  now,
		}).
		Error
}

func (r *Repository) UpdateToolCallResult(ctx context.Context, input domain.ToolCallResultRecordInput) error {
	metadataJSON := ""
	if input.Metadata != nil {
		if b, err := json.Marshal(input.Metadata); err == nil {
			metadataJSON = string(b)
		} else {
			return err
		}
	}
	updates := map[string]any{
		"status":        input.Status,
		"output":        input.Output,
		"error":         input.Error,
		"metadata_json": metadataJSON,
		"updated_at":    time.Now(),
		"finished_at":   input.FinishedAt,
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

func (r *Repository) ListSessionToolCallRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error) {
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
	for start := 0; start < len(filtered); start += 500 {
		var batch []domain.ToolCallRecord
		if err := r.dbWithContext(ctx).Where("session_id = ?", sessionID).Where("assistant_message_id IN ?", filtered[start:min(start+500, len(filtered))]).Order("started_at asc").Order("created_at asc").Find(&batch).Error; err != nil {
			return nil, err
		}
		records = append(records, batch...)
	}
	return records, nil
}
