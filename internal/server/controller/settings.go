package controller

import (
	"net/http"

	settingssvc "slimebot/internal/services/settings"
)

// GetSettings 杩斿洖褰撳墠鍏ㄥ眬璁剧疆锛堝惈榛樿鍊煎洖濉悗鐨勭粨鏋滐級銆?

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
	})
}

// UpdateSettings 鎸夊瓧娈靛閲忔洿鏂板叏灞€璁剧疆銆?

func (h *HTTPController) UpdateSettings(c WebContext) {
	var req struct {
		Language                    string `json:"language"`
		DefaultModel                string `json:"defaultModel"`
		MessagePlatformDefaultModel string `json:"messagePlatformDefaultModel"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	err := h.settings.Update(settingssvc.UpdateSettingsInput{
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
