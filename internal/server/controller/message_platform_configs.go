package controller

import (
	"net/http"

	"slimebot/internal/platforms"
	configsvc "slimebot/internal/services/config"
)

// ListMessagePlatformConfigs 鍒楀嚭鍏ㄩ儴娑堟伅骞冲彴閰嶇疆銆?

func (h *HTTPController) ListMessagePlatformConfigs(c WebContext) {
	items, err := h.platforms.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMessagePlatformConfig 鍒涘缓娑堟伅骞冲彴閰嶇疆骞舵牎楠岄壌鏉冪粨鏋勩€?

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

// UpdateMessagePlatformConfig 鏇存柊娑堟伅骞冲彴閰嶇疆骞跺鐢ㄩ壌鏉冩牎楠屻€?

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

// DeleteMessagePlatformConfig 鍒犻櫎鎸囧畾娑堟伅骞冲彴閰嶇疆銆?

func (h *HTTPController) DeleteMessagePlatformConfig(c WebContext) {
	id := c.Param("id")
	if err := h.platforms.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
