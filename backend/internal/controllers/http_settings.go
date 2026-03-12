package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
