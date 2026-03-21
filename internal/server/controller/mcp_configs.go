package controller

import (
	"net/http"

	configsvc "slimebot/internal/services/config"
)

// ListMCPConfigs 鍒楀嚭鍏ㄩ儴 MCP 鏈嶅姟閰嶇疆銆?

func (h *HTTPController) ListMCPConfigs(c WebContext) {
	items, err := h.mcpConfigs.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMCPConfig 鍒涘缓 MCP 閰嶇疆骞舵墽琛岄厤缃唴瀹规牎楠屻€?

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

// UpdateMCPConfig 鏇存柊鎸囧畾 MCP 閰嶇疆骞堕噸鏂版牎楠屾湁鏁堟€с€?

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

// DeleteMCPConfig 鍒犻櫎鎸囧畾 MCP 閰嶇疆銆?

func (h *HTTPController) DeleteMCPConfig(c WebContext) {
	id := c.Param("id")
	if err := h.mcpConfigs.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
