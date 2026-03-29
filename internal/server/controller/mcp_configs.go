package controller

import (
	"net/http"

	configsvc "slimebot/internal/services/config"
)

// ListMCPConfigs 返回当前保存的全部 MCP 服务配置。
func (h *HTTPController) ListMCPConfigs(c WebContext) {
	items, err := h.mcpConfigs.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMCPConfig 创建 MCP 配置，并在入库前先校验 transport 配置是否合法。
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
	item, err := h.mcpConfigs.Create(configsvc.MCPConfigInput{
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

// UpdateMCPConfig 更新指定 MCP 配置，并复用同一套配置校验逻辑。
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
	if err := h.mcpConfigs.Update(id, configsvc.MCPConfigInput{
		Name:      req.Name,
		Config:    req.Config,
		IsEnabled: req.IsEnabled,
	}); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteMCPConfig 删除指定 MCP 配置。
func (h *HTTPController) DeleteMCPConfig(c WebContext) {
	id := c.Param("id")
	if err := h.mcpConfigs.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
