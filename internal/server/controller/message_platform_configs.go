package controller

import (
	"net/http"

	"slimebot/internal/platforms"
	configsvc "slimebot/internal/services/config"
)

// ListMessagePlatformConfigs returns all message platform configs.
func (h *HTTPController) ListMessagePlatformConfigs(c WebContext) {
	items, err := h.platforms.List(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMessagePlatformConfig creates a platform config and validates auth JSON.
func (h *HTTPController) CreateMessagePlatformConfig(c WebContext) {
	var req struct {
		Platform       string `json:"platform"`
		DisplayName    string `json:"displayName"`
		AuthConfigJSON string `json:"authConfigJson"`
		IsEnabled      bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	req.Platform = lowerTrim(req.Platform)
	trimSpaceFields(&req.DisplayName, &req.AuthConfigJSON)
	if !allFieldsPresent(req.Platform, req.DisplayName, req.AuthConfigJSON) {
		jsonError(c, http.StatusBadRequest, "platform, displayName, and authConfigJson are required.")
		return
	}
	if err := platforms.ValidateAuthConfig(req.Platform, req.AuthConfigJSON); err != nil {
		jsonError(c, http.StatusBadRequest, "authConfigJson is invalid or missing required fields.")
		return
	}
	item, err := h.platforms.Create(c.Request().Context(), configsvc.MessagePlatformConfigInput{
		Platform:       req.Platform,
		DisplayName:    req.DisplayName,
		AuthConfigJSON: req.AuthConfigJSON,
		IsEnabled:      req.IsEnabled,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateMessagePlatformConfig updates a platform config with the same auth validation.
func (h *HTTPController) UpdateMessagePlatformConfig(c WebContext) {
	id := c.Param("id")
	var req struct {
		Platform       string `json:"platform"`
		DisplayName    string `json:"displayName"`
		AuthConfigJSON string `json:"authConfigJson"`
		IsEnabled      bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	req.Platform = lowerTrim(req.Platform)
	trimSpaceFields(&req.DisplayName, &req.AuthConfigJSON)
	if !allFieldsPresent(req.Platform, req.DisplayName, req.AuthConfigJSON) {
		jsonError(c, http.StatusBadRequest, "platform, displayName, and authConfigJson are required.")
		return
	}
	if err := platforms.ValidateAuthConfig(req.Platform, req.AuthConfigJSON); err != nil {
		jsonError(c, http.StatusBadRequest, "authConfigJson is invalid or missing required fields.")
		return
	}
	if err := h.platforms.Update(c.Request().Context(), id, configsvc.MessagePlatformConfigInput{
		Platform:       req.Platform,
		DisplayName:    req.DisplayName,
		AuthConfigJSON: req.AuthConfigJSON,
		IsEnabled:      req.IsEnabled,
	}); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteMessagePlatformConfig removes a message platform config by id.
func (h *HTTPController) DeleteMessagePlatformConfig(c WebContext) {
	id := c.Param("id")
	if err := h.platforms.Delete(c.Request().Context(), id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
