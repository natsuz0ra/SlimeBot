package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type sessionMessagesResponse struct {
	Messages                      []models.Message                    `json:"messages"`
	ToolCallsByAssistantMessageID map[string][]sessionToolCallHistory `json:"toolCallsByAssistantMessageId"`
	HasMore                       bool                                `json:"hasMore"`
}

type sessionToolCallHistory struct {
	ToolCallID       string            `json:"toolCallId"`
	ToolName         string            `json:"toolName"`
	Command          string            `json:"command"`
	Params           map[string]string `json:"params"`
	Status           string            `json:"status"`
	RequiresApproval bool              `json:"requiresApproval"`
	Output           string            `json:"output,omitempty"`
	Error            string            `json:"error,omitempty"`
	StartedAt        string            `json:"startedAt"`
	FinishedAt       string            `json:"finishedAt,omitempty"`
}

// parseToolCallParams 解析 tool_call 参数 JSON；异常时回退为空对象避免前端崩溃。
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

// ListSessions 返回当前用户的会话列表。
func (h *HTTPController) ListSessions(c *gin.Context) {
	sessions, err := h.sessions.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessions)
}

// CreateSession 创建会话；未传 name 时使用默认名称。
func (h *HTTPController) CreateSession(c *gin.Context) {
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

// RenameSession 修改指定会话名称。
func (h *HTTPController) RenameSession(c *gin.Context) {
	id := c.Param("id")
	if id == consts.MessagePlatformSessionID {
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

// DeleteSession 删除指定会话及其关联数据。
func (h *HTTPController) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if id == consts.MessagePlatformSessionID {
		jsonError(c, http.StatusBadRequest, "Message platform sessions cannot be deleted.")
		return
	}
	if err := h.sessions.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMessages 返回会话消息，并附带 assistant 消息关联的工具调用历史。
func (h *HTTPController) ListMessages(c *gin.Context) {
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

	messages, hasMore, err := h.sessions.ListMessagesPage(sessionID, limit, before, after)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	messageIDSet := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		messageIDSet[message.ID] = struct{}{}
	}
	records, err := h.sessions.ListToolCallRecords(sessionID)
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
			Output:           record.Output,
			Error:            record.Error,
			StartedAt:        record.StartedAt.Format("2006-01-02T15:04:05.000Z07:00"),
		}
		if record.FinishedAt != nil {
			item.FinishedAt = record.FinishedAt.Format("2006-01-02T15:04:05.000Z07:00")
		}
		toolCallsByAssistantMessageID[key] = append(toolCallsByAssistantMessageID[key], item)
	}
	c.JSON(http.StatusOK, sessionMessagesResponse{
		Messages:                      messages,
		ToolCallsByAssistantMessageID: toolCallsByAssistantMessageID,
		HasMore:                       hasMore,
	})
}

// SetSessionModel 设置会话默认模型配置。
func (h *HTTPController) SetSessionModel(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		ModelConfigID string `json:"modelConfigId" binding:"required"`
	}
	if !bindJSONOrBadRequest(c, &req, "modelConfigId is required.") {
		return
	}
	if err := h.sessions.SetModel(id, req.ModelConfigID); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
