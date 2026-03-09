package controllers

import (
	"net/http"
	"strings"

	"corner/backend/internal/models"
	"corner/backend/internal/repositories"
	"github.com/gin-gonic/gin"
)

type HTTPController struct {
	repo *repositories.Repository
}

func NewHTTPController(repo *repositories.Repository) *HTTPController {
	return &HTTPController{repo: repo}
}

func (h *HTTPController) ListSessions(c *gin.Context) {
	sessions, err := h.repo.ListSessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sessions)
}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *HTTPController) RenameSession(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 必填"})
		return
	}
	if err := h.repo.RenameSessionByUser(id, strings.TrimSpace(req.Name)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.DeleteSession(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) ListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	messages, err := h.repo.ListSessionMessages(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (h *HTTPController) SetSessionModel(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		ModelConfigID string `json:"modelConfigId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modelConfigId 必填"})
		return
	}
	if err := h.repo.SetSessionModel(id, req.ModelConfigID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) GetSettings(c *gin.Context) {
	language, err := h.repo.GetSetting("language")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if language == "" {
		language = "zh-CN"
	}

	defaultModel, err := h.repo.GetSetting("defaultModel")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"language":     language,
		"defaultModel": defaultModel,
	})
}

func (h *HTTPController) UpdateSettings(c *gin.Context) {
	var req struct {
		Language     string `json:"language"`
		DefaultModel string `json:"defaultModel"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	if req.Language != "" {
		if err := h.repo.SetSetting("language", req.Language); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.DefaultModel != "" {
		if err := h.repo.SetSetting("defaultModel", req.DefaultModel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) ListLLMConfigs(c *gin.Context) {
	items, err := h.repo.ListLLMConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPController) CreateLLMConfig(c *gin.Context) {
	var req models.LLMConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" || req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name/baseUrl/apiKey/model 均必填"})
		return
	}
	item, err := h.repo.CreateLLMConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *HTTPController) DeleteLLMConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.DeleteLLMConfig(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) ListMCPConfigs(c *gin.Context) {
	items, err := h.repo.ListMCPConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPController) CreateMCPConfig(c *gin.Context) {
	var req models.MCPConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	if req.Name == "" || req.ServerURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name/serverUrl 必填"})
		return
	}
	item, err := h.repo.CreateMCPConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *HTTPController) UpdateMCPConfig(c *gin.Context) {
	id := c.Param("id")
	var req models.MCPConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	if err := h.repo.UpdateMCPConfig(id, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) DeleteMCPConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.DeleteMCPConfig(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
