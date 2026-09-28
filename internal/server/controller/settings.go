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
		"language":                        settings.Language,
		"defaultModel":                    settings.DefaultModel,
		"messagePlatformDefaultModel":     settings.MessagePlatformDefaultModel,
		"messagePlatformThinkingLevel":    settings.MessagePlatformThinkingLevel,
		"messagePlatformApprovalMode":     settings.MessagePlatformApprovalMode,
		"webSearchApiKey":                 settings.WebSearchAPIKey,
		"proxyUrl":                        settings.ProxyURL,
		"approvalMode":                    settings.ApprovalMode,
		"thinkingLevel":                   settings.ThinkingLevel,
		"sandboxMode":                     settings.SandboxMode,
		"sandboxWritableRoots":            settings.SandboxWritableRoots,
		"sandboxNetworkEnabled":           settings.SandboxNetworkEnabled,
		"sandboxNetworkAllowedDomains":    settings.SandboxNetworkAllowedDomains,
		"cliSandboxMode":                  settings.CLISandboxMode,
		"cliSandboxWritableRoots":         settings.CLISandboxWritableRoots,
		"cliSandboxNetworkEnabled":        settings.CLISandboxNetworkEnabled,
		"cliSandboxNetworkAllowedDomains": settings.CLISandboxNetworkAllowedDomains,
		"memoryEnabled":                   settings.MemoryEnabled,
		"memoryUserProfileEnabled":        settings.MemoryUserProfileEnabled,
		"memoryCharLimit":                 settings.MemoryCharLimit,
		"memoryUserCharLimit":             settings.MemoryUserCharLimit,
		"memoryNudgeInterval":             settings.MemoryNudgeInterval,
	})
}

// UpdateSettings patches global settings by field.
func (h *HTTPController) UpdateSettings(c WebContext) {
	var req struct {
		Language                        *string   `json:"language"`
		DefaultModel                    *string   `json:"defaultModel"`
		MessagePlatformDefaultModel     *string   `json:"messagePlatformDefaultModel"`
		MessagePlatformThinkingLevel    *string   `json:"messagePlatformThinkingLevel"`
		MessagePlatformApprovalMode     *string   `json:"messagePlatformApprovalMode"`
		WebSearchAPIKey                 *string   `json:"webSearchApiKey"`
		ProxyURL                        *string   `json:"proxyUrl"`
		ApprovalMode                    *string   `json:"approvalMode"`
		ThinkingLevel                   *string   `json:"thinkingLevel"`
		SandboxMode                     *string   `json:"sandboxMode"`
		SandboxWritableRoots            *[]string `json:"sandboxWritableRoots"`
		SandboxNetworkEnabled           *bool     `json:"sandboxNetworkEnabled"`
		SandboxNetworkAllowedDomains    *[]string `json:"sandboxNetworkAllowedDomains"`
		CLISandboxMode                  *string   `json:"cliSandboxMode"`
		CLISandboxWritableRoots         *[]string `json:"cliSandboxWritableRoots"`
		CLISandboxNetworkEnabled        *bool     `json:"cliSandboxNetworkEnabled"`
		CLISandboxNetworkAllowedDomains *[]string `json:"cliSandboxNetworkAllowedDomains"`
		MemoryEnabled                   *bool     `json:"memoryEnabled"`
		MemoryUserProfileEnabled        *bool     `json:"memoryUserProfileEnabled"`
		MemoryCharLimit                 *int      `json:"memoryCharLimit"`
		MemoryUserCharLimit             *int      `json:"memoryUserCharLimit"`
		MemoryNudgeInterval             *int      `json:"memoryNudgeInterval"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	err := h.settings.Update(c.Request().Context(), settingssvc.UpdateSettingsInput{
		Language:                        req.Language,
		DefaultModel:                    req.DefaultModel,
		MessagePlatformDefaultModel:     req.MessagePlatformDefaultModel,
		MessagePlatformThinkingLevel:    req.MessagePlatformThinkingLevel,
		MessagePlatformApprovalMode:     req.MessagePlatformApprovalMode,
		WebSearchAPIKey:                 req.WebSearchAPIKey,
		ProxyURL:                        req.ProxyURL,
		ApprovalMode:                    req.ApprovalMode,
		ThinkingLevel:                   req.ThinkingLevel,
		SandboxMode:                     req.SandboxMode,
		SandboxWritableRoots:            req.SandboxWritableRoots,
		SandboxNetworkEnabled:           req.SandboxNetworkEnabled,
		SandboxNetworkAllowedDomains:    req.SandboxNetworkAllowedDomains,
		CLISandboxMode:                  req.CLISandboxMode,
		CLISandboxWritableRoots:         req.CLISandboxWritableRoots,
		CLISandboxNetworkEnabled:        req.CLISandboxNetworkEnabled,
		CLISandboxNetworkAllowedDomains: req.CLISandboxNetworkAllowedDomains,
		MemoryEnabled:                   req.MemoryEnabled,
		MemoryUserProfileEnabled:        req.MemoryUserProfileEnabled,
		MemoryCharLimit:                 req.MemoryCharLimit,
		MemoryUserCharLimit:             req.MemoryUserCharLimit,
		MemoryNudgeInterval:             req.MemoryNudgeInterval,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
