package controller

import (
	"net/http"

	"slimebot/internal/platforms"
	configsvc "slimebot/internal/services/config"
)

// ListMessagePlatformConfigs 返回全部消息平台接入配置。
func (h *HTTPController) ListMessagePlatformConfigs(c WebContext) {
	items, err := h.platforms.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMessagePlatformConfig 创建消息平台配置，并校验平台鉴权 JSON。
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
	item, err := h.platforms.Create(configsvc.MessagePlatformConfigInput{
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

// UpdateMessagePlatformConfig 更新消息平台配置，并复用鉴权配置校验。
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
	if err := h.platforms.Update(id, configsvc.MessagePlatformConfigInput{
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

// DeleteMessagePlatformConfig 删除指定消息平台配置。
func (h *HTTPController) DeleteMessagePlatformConfig(c WebContext) {
	id := c.Param("id")
	if err := h.platforms.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
