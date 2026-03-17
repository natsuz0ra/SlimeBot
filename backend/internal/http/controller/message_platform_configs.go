package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/services"
)

type telegramAuthConfig struct {
	BotToken string `json:"botToken"`
}

// validatePlatformAuthConfig 校验平台鉴权 JSON，避免写入不可用配置。
func validatePlatformAuthConfig(platform string, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("auth config is empty")
	}
	var asObject map[string]any
	if err := json.Unmarshal([]byte(trimmed), &asObject); err != nil {
		return err
	}
	if strings.EqualFold(platform, consts.TelegramPlatformName) {
		var cfg telegramAuthConfig
		if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
			return err
		}
		if strings.TrimSpace(cfg.BotToken) == "" {
			return fmt.Errorf("telegram botToken is required")
		}
	}
	return nil
}

// ListMessagePlatformConfigs 列出全部消息平台配置。
func (h *HTTPController) ListMessagePlatformConfigs(c *gin.Context) {
	items, err := h.platforms.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMessagePlatformConfig 创建消息平台配置并校验鉴权结构。
func (h *HTTPController) CreateMessagePlatformConfig(c *gin.Context) {
	var req struct {
		Platform       string `json:"platform"`
		DisplayName    string `json:"displayName"`
		AuthConfigJSON string `json:"authConfigJson"`
		IsEnabled      bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.AuthConfigJSON = strings.TrimSpace(req.AuthConfigJSON)
	if req.Platform == "" || req.DisplayName == "" || req.AuthConfigJSON == "" {
		jsonError(c, http.StatusBadRequest, "platform, displayName, and authConfigJson are required.")
		return
	}
	if err := validatePlatformAuthConfig(req.Platform, req.AuthConfigJSON); err != nil {
		jsonError(c, http.StatusBadRequest, "authConfigJson is invalid or missing required fields.")
		return
	}
	item, err := h.platforms.Create(services.MessagePlatformConfigInput{
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

// UpdateMessagePlatformConfig 更新消息平台配置并复用鉴权校验。
func (h *HTTPController) UpdateMessagePlatformConfig(c *gin.Context) {
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
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.AuthConfigJSON = strings.TrimSpace(req.AuthConfigJSON)
	if req.Platform == "" || req.DisplayName == "" || req.AuthConfigJSON == "" {
		jsonError(c, http.StatusBadRequest, "platform, displayName, and authConfigJson are required.")
		return
	}
	if err := validatePlatformAuthConfig(req.Platform, req.AuthConfigJSON); err != nil {
		jsonError(c, http.StatusBadRequest, "authConfigJson is invalid or missing required fields.")
		return
	}
	if err := h.platforms.Update(id, services.MessagePlatformConfigInput{
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
func (h *HTTPController) DeleteMessagePlatformConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.platforms.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
