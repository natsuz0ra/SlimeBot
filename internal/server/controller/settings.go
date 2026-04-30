package controller

import (
	"net/http"

	settingssvc "slimebot/internal/services/settings"
)

// GetSettings returns global settings with service-layer defaults.
func (h *HTTPController) GetSettings(c WebContext) {
	settings, err := h.settings.Get(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"language":                    settings.Language,
		"defaultModel":                settings.DefaultModel,
		"messagePlatformDefaultModel": settings.MessagePlatformDefaultModel,
		"webSearchApiKey":             settings.WebSearchAPIKey,
		"approvalMode":                settings.ApprovalMode,
		"thinkingLevel":               settings.ThinkingLevel,
	})
}

// UpdateSettings patches global settings by field.
func (h *HTTPController) UpdateSettings(c WebContext) {
	var req struct {
		Language                    string `json:"language"`
		DefaultModel                string `json:"defaultModel"`
		MessagePlatformDefaultModel string `json:"messagePlatformDefaultModel"`
		WebSearchAPIKey             string `json:"webSearchApiKey"`
		ApprovalMode                string `json:"approvalMode"`
		ThinkingLevel               string `json:"thinkingLevel"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	err := h.settings.Update(c.Request().Context(), settingssvc.UpdateSettingsInput{
		Language:                    req.Language,
		DefaultModel:                req.DefaultModel,
		MessagePlatformDefaultModel: req.MessagePlatformDefaultModel,
		WebSearchAPIKey:             req.WebSearchAPIKey,
		ApprovalMode:                req.ApprovalMode,
		ThinkingLevel:               req.ThinkingLevel,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
