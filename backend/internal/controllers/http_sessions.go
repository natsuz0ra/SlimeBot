package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

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
