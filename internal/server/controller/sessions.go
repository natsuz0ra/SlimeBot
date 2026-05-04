package controller

import (
	"errors"
	"io"
	"net/http"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	sessionsvc "slimebot/internal/services/session"
	"strconv"
	"strings"
	"time"

	"slimebot/internal/constants"
)

type sessionMessagesResponse struct {
	Messages                        []domain.Message                        `json:"messages"`
	ToolCallsByAssistantMessageID   map[string][]sessionsvc.ToolCallHistory `json:"toolCallsByAssistantMessageId"`
	ThinkingByAssistantMessageID    map[string][]sessionsvc.ThinkingHistory `json:"thinkingByAssistantMessageId"`
	ReplyTimingByAssistantMessageID map[string]sessionsvc.ReplyTiming       `json:"replyTimingByAssistantMessageId"`
	HasMore                         bool                                    `json:"hasMore"`
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
	result, err := h.sessions.List(c.Request().Context(), limit, offset, q)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, listSessionsResponse{Sessions: result.Sessions, HasMore: result.HasMore})
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
	session, err := h.sessions.Create(c.Request().Context(), req.Name)
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
	if err := h.sessions.RenameByUser(c.Request().Context(), id, req.Name); err != nil {
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
	if err := h.sessions.Delete(c.Request().Context(), id); err != nil {
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

	history, err := h.sessions.GetMessageHistory(c.Request().Context(), sessionID, limit, before, beforeSeq, after, afterSeq)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionMessagesResponse{
		Messages:                        history.Messages,
		ToolCallsByAssistantMessageID:   history.ToolCallsByAssistantMessageID,
		ThinkingByAssistantMessageID:    history.ThinkingByAssistantMessageID,
		ReplyTimingByAssistantMessageID: history.ReplyTimingByAssistantMessageID,
		HasMore:                         history.HasMore,
	})
	logging.Span("http_list_messages", listStart)
}

func (h *HTTPController) GetContextUsage(c WebContext) {
	if h.chatUsage == nil {
		jsonError(c, http.StatusServiceUnavailable, "Context usage service is unavailable.")
		return
	}
	sessionID := c.Param("id")
	modelID := strings.TrimSpace(c.Query("modelId"))
	if modelID == "" {
		jsonError(c, http.StatusBadRequest, "modelId is required.")
		return
	}
	usage, err := h.chatUsage.GetContextUsage(c.Request().Context(), sessionID, modelID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, usage)
}
