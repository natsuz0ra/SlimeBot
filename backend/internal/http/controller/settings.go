package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/services"
)

// GetSettings 返回当前全局设置（含默认值回填后的结果）。
func (h *HTTPController) GetSettings(c *gin.Context) {
	settings, err := h.settings.Get()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"language":                    settings.Language,
		"defaultModel":                settings.DefaultModel,
		"messagePlatformDefaultModel": settings.MessagePlatformDefaultModel,
	})
}

// UpdateSettings 按字段增量更新全局设置。
func (h *HTTPController) UpdateSettings(c *gin.Context) {
	var req struct {
		Language                    string `json:"language"`
		DefaultModel                string `json:"defaultModel"`
		MessagePlatformDefaultModel string `json:"messagePlatformDefaultModel"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	err := h.settings.Update(services.UpdateSettingsInput{
		Language:                    req.Language,
		DefaultModel:                req.DefaultModel,
		MessagePlatformDefaultModel: req.MessagePlatformDefaultModel,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
