package controller

import (
	"net/http"

	configsvc "slimebot/internal/services/config"
)

// ListMCPConfigs returns all saved MCP server configs.
func (h *HTTPController) ListMCPConfigs(c WebContext) {
	items, err := h.mcpConfigs.List(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMCPConfig creates an MCP config after validating transport settings.
func (h *HTTPController) CreateMCPConfig(c WebContext) {
	var req struct {
		Name      string `json:"name"`
		Config    string `json:"config"`
		IsEnabled bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	trimSpaceFields(&req.Name, &req.Config)
	if !allFieldsPresent(req.Name, req.Config) {
		jsonError(c, http.StatusBadRequest, "Both name and config are required.")
		return
	}
	if err := h.mcpConfigs.ValidateConfig(req.Config); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.mcpConfigs.Create(c.Request().Context(), configsvc.MCPConfigInput{
		Name:      req.Name,
		Config:    req.Config,
		IsEnabled: req.IsEnabled,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateMCPConfig updates an MCP config using the same validation as create.
func (h *HTTPController) UpdateMCPConfig(c WebContext) {
	id := c.Param("id")
	var req struct {
		Name      string `json:"name"`
		Config    string `json:"config"`
		IsEnabled bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	trimSpaceFields(&req.Name, &req.Config)
	if !allFieldsPresent(req.Name, req.Config) {
		jsonError(c, http.StatusBadRequest, "Both name and config are required.")
		return
	}
	if err := h.mcpConfigs.ValidateConfig(req.Config); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.mcpConfigs.Update(c.Request().Context(), id, configsvc.MCPConfigInput{
		Name:      req.Name,
		Config:    req.Config,
		IsEnabled: req.IsEnabled,
	}); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteMCPConfig removes an MCP config by id.
func (h *HTTPController) DeleteMCPConfig(c WebContext) {
	id := c.Param("id")
	if err := h.mcpConfigs.Delete(c.Request().Context(), id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
