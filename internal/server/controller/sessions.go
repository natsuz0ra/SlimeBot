package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"strconv"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/repositories"
)

type sessionMessagesResponse struct {
	Messages                      []domain.Message                    `json:"messages"`
	ToolCallsByAssistantMessageID map[string][]sessionToolCallHistory `json:"toolCallsByAssistantMessageId"`
	ThinkingByAssistantMessageID  map[string][]sessionThinkingHistory `json:"thinkingByAssistantMessageId"`
	HasMore                       bool                                `json:"hasMore"`
}

type sessionToolCallHistory struct {
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

type sessionThinkingHistory struct {
	ThinkingID string `json:"thinkingId"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// parseToolCallParams parses tool_call params JSON; on error returns empty map to avoid client crashes.
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

func parseSessionMessagesCursor(raw string) (*time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

type listSessionsResponse struct {
	Sessions []domain.Session `json:"sessions"`
	HasMore  bool             `json:"hasMore"`
}

// ListSessions returns the current user's sessions.
func (h *HTTPController) ListSessions(c WebContext) {
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			jsonError(c, http.StatusBadRequest, "limit must be a positive integer.")
			return
		}
		if parsed > 500 {
			parsed = 500
		}
		limit = parsed
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			jsonError(c, http.StatusBadRequest, "offset must be a non-negative integer.")
			return
		}
		offset = parsed
	}
	q := strings.TrimSpace(c.Query("q"))
	sessions, err := h.sessions.List(limit+1, offset, q)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	sessions, hasMore := repositories.FetchWindow(sessions, limit)
	c.JSON(http.StatusOK, listSessionsResponse{Sessions: sessions, HasMore: hasMore})
}

// CreateSession creates a session; default name is used when name is omitted.
func (h *HTTPController) CreateSession(c WebContext) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		jsonError(c, http.StatusBadRequest, "Invalid request payload format.")
		return
	}
	session, err := h.sessions.Create(req.Name)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

// RenameSession renames a session.
func (h *HTTPController) RenameSession(c WebContext) {
	id := c.Param("id")
	if id == constants.MessagePlatformSessionID {
		jsonError(c, http.StatusBadRequest, "Message platform sessions cannot be renamed.")
		return
	}
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if !bindJSONOrBadRequest(c, &req, "name is required.") {
		return
	}
	if err := h.sessions.RenameByUser(id, req.Name); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteSession deletes a session and related rows.
func (h *HTTPController) DeleteSession(c WebContext) {
	id := c.Param("id")
	if id == constants.MessagePlatformSessionID {
		jsonError(c, http.StatusBadRequest, "Message platform sessions cannot be deleted.")
		return
	}
	if err := h.sessions.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMessages returns session messages plus tool-call history keyed by assistant message id.
func (h *HTTPController) ListMessages(c WebContext) {
	listStart := time.Now()
	sessionID := c.Param("id")
	limit := 10
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit <= 0 {
			jsonError(c, http.StatusBadRequest, "limit must be a positive integer.")
			return
		}
		if parsedLimit > 50 {
			parsedLimit = 50
		}
		limit = parsedLimit
	}
	before, ok := parseSessionMessagesCursor(c.Query("before"))
	if !ok {
		jsonError(c, http.StatusBadRequest, "before must be RFC3339 format.")
		return
	}
	after, ok := parseSessionMessagesCursor(c.Query("after"))
	if !ok {
		jsonError(c, http.StatusBadRequest, "after must be RFC3339 format.")
		return
	}
	if before != nil && after != nil {
		jsonError(c, http.StatusBadRequest, "before and after cannot be used together.")
		return
	}

	var beforeSeq *int64
	if raw := strings.TrimSpace(c.Query("beforeSeq")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			jsonError(c, http.StatusBadRequest, "beforeSeq must be an integer.")
			return
		}
		beforeSeq = &v
	}
	var afterSeq *int64
	if raw := strings.TrimSpace(c.Query("afterSeq")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			jsonError(c, http.StatusBadRequest, "afterSeq must be an integer.")
			return
		}
		afterSeq = &v
	}
	if before != nil && beforeSeq == nil {
		jsonError(c, http.StatusBadRequest, "beforeSeq is required when before is set.")
		return
	}
	if after != nil && afterSeq == nil {
		jsonError(c, http.StatusBadRequest, "afterSeq is required when after is set.")
		return
	}

	messages, hasMore, err := h.sessions.ListMessagesPage(sessionID, limit, before, beforeSeq, after, afterSeq)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	messageIDSet := make(map[string]struct{}, len(messages))
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		messageIDSet[message.ID] = struct{}{}
		messageIDs = append(messageIDs, message.ID)
	}
	records, err := h.sessions.ListToolCallRecordsByAssistantMessageIDs(sessionID, messageIDs)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	thinkingRecords, err := h.sessions.ListThinkingRecordsByAssistantMessageIDs(sessionID, messageIDs)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	toolCallsByAssistantMessageID := make(map[string][]sessionToolCallHistory)
	for _, record := range records {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		item := sessionToolCallHistory{
			ToolCallID:       record.ToolCallID,
			ToolName:         record.ToolName,
			Command:          record.Command,
			Params:           parseToolCallParams(record.ParamsJSON),
			Status:           record.Status,
			RequiresApproval: record.RequiresApproval,
			ParentToolCallID: record.ParentToolCallID,
			SubagentRunID:    record.SubagentRunID,
			Output:           record.Output,
			Error:            record.Error,
			StartedAt:        record.StartedAt.Format("2006-01-02T15:04:05.000Z07:00"),
		}
		if record.FinishedAt != nil {
			item.FinishedAt = record.FinishedAt.Format("2006-01-02T15:04:05.000Z07:00")
		}
		toolCallsByAssistantMessageID[key] = append(toolCallsByAssistantMessageID[key], item)
	}
	thinkingByAssistantMessageID := make(map[string][]sessionThinkingHistory)
	for _, record := range thinkingRecords {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		if _, ok := messageIDSet[key]; !ok {
			continue
		}
		item := sessionThinkingHistory{
			ThinkingID: record.ThinkingID,
			Content:    record.Content,
			Status:     record.Status,
			StartedAt:  record.StartedAt.Format("2006-01-02T15:04:05.000Z07:00"),
			DurationMs: record.DurationMs,
		}
		if record.FinishedAt != nil {
			item.FinishedAt = record.FinishedAt.Format("2006-01-02T15:04:05.000Z07:00")
		}
		thinkingByAssistantMessageID[key] = append(thinkingByAssistantMessageID[key], item)
	}
	c.JSON(http.StatusOK, sessionMessagesResponse{
		Messages:                      messages,
		ToolCallsByAssistantMessageID: toolCallsByAssistantMessageID,
		ThinkingByAssistantMessageID:  thinkingByAssistantMessageID,
		HasMore:                       hasMore,
	})
	logging.Span("http_list_messages", listStart)
}
