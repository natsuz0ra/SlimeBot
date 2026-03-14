package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/models"
)

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
	req.Name = strings.TrimSpace(req.Name)
	req.Config = strings.TrimSpace(req.Config)
	if req.Name == "" || req.Config == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name/config 必填"})
		return
	}
	if _, err := mcp.ParseAndValidateConfig(req.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	req.Name = strings.TrimSpace(req.Name)
	req.Config = strings.TrimSpace(req.Config)
	if req.Name == "" || req.Config == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name/config 必填"})
		return
	}
	if _, err := mcp.ParseAndValidateConfig(req.Config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
