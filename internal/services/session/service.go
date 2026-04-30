package session

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// SessionService orchestrates session use cases; controllers stay thin.
type SessionService struct {
	store domain.SessionStore
}

func NewSessionService(store domain.SessionStore) *SessionService {
	return &SessionService{store: store}
}

type ListResult struct {
	Sessions []domain.Session
	HasMore  bool
}

type MessageHistoryPage struct {
	Messages                        []domain.Message
	ToolCallsByAssistantMessageID   map[string][]ToolCallHistory
	ThinkingByAssistantMessageID    map[string][]ThinkingHistory
	ReplyTimingByAssistantMessageID map[string]ReplyTiming
	HasMore                         bool
}

type ToolCallHistory struct {
	ToolCallID       string            `json:"toolCallId"`
	ToolName         string            `json:"toolName"`
	Command          string            `json:"command"`
	Params           map[string]string `json:"params"`
	Status           string            `json:"status"`
	RequiresApproval bool              `json:"requiresApproval"`
	ParentToolCallID string            `json:"parentToolCallId,omitempty"`
	SubagentRunID    string            `json:"subagentRunId,omitempty"`
	Output           string            `json:"output,omitempty"`
	Error            string            `json:"error,omitempty"`
	StartedAt        string            `json:"startedAt"`
	FinishedAt       string            `json:"finishedAt,omitempty"`
}

type ThinkingHistory struct {
	ThinkingID       string `json:"thinkingId"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`
	SubagentRunID    string `json:"subagentRunId,omitempty"`
	Content          string `json:"content"`
	Status           string `json:"status"`
	StartedAt        string `json:"startedAt"`
	FinishedAt       string `json:"finishedAt,omitempty"`
	DurationMs       int64  `json:"durationMs"`
}

type ReplyTiming struct {
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	DurationMs int64  `json:"durationMs"`
}

func (s *SessionService) List(ctx context.Context, limit, offset int, query string) (ListResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	sessions, err := s.store.ListSessions(ctx, limit+1, offset, strings.TrimSpace(query))
	if err != nil {
		return ListResult{}, err
	}
	sessions, hasMore := fetchWindow(sessions, limit)
	return ListResult{Sessions: sessions, HasMore: hasMore}, nil
}

func (s *SessionService) Create(ctx context.Context, name string) (*domain.Session, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "New Chat"
	}
	return s.store.CreateSession(ctx, trimmed)
}

func (s *SessionService) RenameByUser(ctx context.Context, id string, name string) error {
	return s.store.RenameSessionByUser(ctx, id, strings.TrimSpace(name))
}

func (s *SessionService) Delete(ctx context.Context, id string) error {
	return s.store.DeleteSession(ctx, id)
}

func (s *SessionService) GetMessageHistory(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) (MessageHistoryPage, error) {
	messages, hasMore, err := s.store.ListSessionMessagesPage(ctx, sessionID, limit, before, beforeSeq, after, afterSeq)
	if err != nil {
		return MessageHistoryPage{}, err
	}
	messageIDSet := make(map[string]struct{}, len(messages))
	interruptedAssistantIDs := make(map[string]struct{}, len(messages))
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		messageIDSet[message.ID] = struct{}{}
		if message.Role == "assistant" && message.IsInterrupted {
			interruptedAssistantIDs[message.ID] = struct{}{}
		}
		messageIDs = append(messageIDs, message.ID)
	}

	records, err := s.store.ListSessionToolCallRecordsByAssistantMessageIDs(ctx, sessionID, messageIDs)
	if err != nil {
		return MessageHistoryPage{}, err
	}
	thinkingRecords, err := s.store.ListSessionThinkingRecordsByAssistantMessageIDs(ctx, sessionID, messageIDs)
	if err != nil {
		return MessageHistoryPage{}, err
	}

	return MessageHistoryPage{
		Messages:                        messages,
		ToolCallsByAssistantMessageID:   buildToolCallHistory(records, messageIDSet, interruptedAssistantIDs),
		ThinkingByAssistantMessageID:    buildThinkingHistory(thinkingRecords, messageIDSet, interruptedAssistantIDs),
		ReplyTimingByAssistantMessageID: buildReplyTiming(messages),
		HasMore:                         hasMore,
	}, nil
}

func fetchWindow[T any](items []T, limit int) (trimmed []T, hasMore bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func formatHistoryTime(value time.Time) string {
	return value.Format("2006-01-02T15:04:05.000Z07:00")
}

func parseToolCallParams(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]string{}
	}
	var params map[string]string
	if err := json.Unmarshal([]byte(trimmed), &params); err != nil {
		return map[string]string{}
	}
	return params
}

func buildToolCallHistory(records []domain.ToolCallRecord, messageIDSet, interruptedAssistantIDs map[string]struct{}) map[string][]ToolCallHistory {
	byAssistantID := make(map[string][]ToolCallHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		status := record.Status
		errText := record.Error
		if _, interrupted := interruptedAssistantIDs[key]; interrupted && (status == constants.ToolCallStatusPending || status == constants.ToolCallStatusExecuting) {
			status = constants.ToolCallStatusError
			if strings.TrimSpace(errText) == "" {
				errText = "Execution cancelled."
			}
		}
		item := ToolCallHistory{
			ToolCallID:       record.ToolCallID,
			ToolName:         record.ToolName,
			Command:          record.Command,
			Params:           parseToolCallParams(record.ParamsJSON),
			Status:           status,
			RequiresApproval: record.RequiresApproval,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Output:           record.Output,
			Error:            errText,
			StartedAt:        formatHistoryTime(record.StartedAt),
		}
		if record.FinishedAt != nil {
			item.FinishedAt = formatHistoryTime(*record.FinishedAt)
		}
		byAssistantID[key] = append(byAssistantID[key], item)
	}
	return byAssistantID
}

func buildThinkingHistory(records []domain.ThinkingRecord, messageIDSet, interruptedAssistantIDs map[string]struct{}) map[string][]ThinkingHistory {
	byAssistantID := make(map[string][]ThinkingHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		status := record.Status
		if _, interrupted := interruptedAssistantIDs[key]; interrupted && status == "streaming" {
			status = "completed"
		}
		item := ThinkingHistory{
			ThinkingID:       record.ThinkingID,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Content:          record.Content,
			Status:           status,
			StartedAt:        formatHistoryTime(record.StartedAt),
			DurationMs:       record.DurationMs,
		}
		if record.FinishedAt != nil {
			item.FinishedAt = formatHistoryTime(*record.FinishedAt)
		}
		byAssistantID[key] = append(byAssistantID[key], item)
	}
	return byAssistantID
}

func buildReplyTiming(messages []domain.Message) map[string]ReplyTiming {
	byAssistantID := make(map[string]ReplyTiming)
	var previousUser *domain.Message
	for idx := range messages {
		message := messages[idx]
		switch message.Role {
		case "user":
			previousUser = &messages[idx]
		case "assistant":
			if previousUser == nil {
				continue
			}
			durationMs := message.CreatedAt.Sub(previousUser.CreatedAt).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
			byAssistantID[message.ID] = ReplyTiming{
				StartedAt:  formatHistoryTime(previousUser.CreatedAt),
				FinishedAt: formatHistoryTime(message.CreatedAt),
				DurationMs: durationMs,
			}
			previousUser = nil
		}
	}
	return byAssistantID
}
