package controllers

import (
	"encoding/json"
	"net/http"
	"strings"

	"slimebot/backend/internal/models"

	"github.com/gin-gonic/gin"
)

type sessionMessagesResponse struct {
	Messages                      []models.Message                    `json:"messages"`
	ToolCallsByAssistantMessageID map[string][]sessionToolCallHistory `json:"toolCallsByAssistantMessageId"`
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

// ListSessions 返回当前用户的会话列表。
func (h *HTTPController) ListSessions(c *gin.Context) {
	sessions, err := h.repo.ListSessions()
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
	_ = c.ShouldBindJSON(&req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "新会话"
	}
	session, err := h.repo.CreateSession(name)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

// RenameSession 修改指定会话名称。
func (h *HTTPController) RenameSession(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if !bindJSONOrBadRequest(c, &req, "name 必填") {
		return
	}
	if err := h.repo.RenameSessionByUser(id, strings.TrimSpace(req.Name)); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteSession 删除指定会话及其关联数据。
func (h *HTTPController) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.DeleteSession(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMessages 返回会话消息，并附带 assistant 消息关联的工具调用历史。
func (h *HTTPController) ListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	messages, err := h.repo.ListSessionMessages(sessionID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	records, err := h.repo.ListSessionToolCallRecords(sessionID)
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
	})
}

// SetSessionModel 设置会话默认模型配置。
func (h *HTTPController) SetSessionModel(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		ModelConfigID string `json:"modelConfigId" binding:"required"`
	}
	if !bindJSONOrBadRequest(c, &req, "modelConfigId 必填") {
		return
	}
	if err := h.repo.SetSessionModel(id, req.ModelConfigID); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
