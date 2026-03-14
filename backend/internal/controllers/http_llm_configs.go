package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/models"
)

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
