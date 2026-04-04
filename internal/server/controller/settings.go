package controller

import (
	"net/http"

	settingssvc "slimebot/internal/services/settings"
)

// GetSettings 返回当前全局设置，并补齐服务层默认值。
func (h *HTTPController) GetSettings(c WebContext) {
	settings, err := h.settings.Get()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"language":                    settings.Language,
		"defaultModel":                settings.DefaultModel,
		"messagePlatformDefaultModel": settings.MessagePlatformDefaultModel,
		"webSearchApiKey":             settings.WebSearchAPIKey,
	})
}

// UpdateSettings 按字段更新全局设置。
func (h *HTTPController) UpdateSettings(c WebContext) {
	var req struct {
		Language                    string `json:"language"`
		DefaultModel                string `json:"defaultModel"`
		MessagePlatformDefaultModel string `json:"messagePlatformDefaultModel"`
		WebSearchAPIKey             string `json:"webSearchApiKey"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	err := h.settings.Update(settingssvc.UpdateSettingsInput{
		Language:                    req.Language,
		DefaultModel:                req.DefaultModel,
		MessagePlatformDefaultModel: req.MessagePlatformDefaultModel,
		WebSearchAPIKey:             req.WebSearchAPIKey,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
