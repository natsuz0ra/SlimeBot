package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/services"
)

// ListMCPConfigs 列出全部 MCP 服务配置。
func (h *HTTPController) ListMCPConfigs(c *gin.Context) {
	items, err := h.mcpConfigs.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMCPConfig 创建 MCP 配置并执行配置内容校验。
func (h *HTTPController) CreateMCPConfig(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		Config    string `json:"config"`
		IsEnabled bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Config = strings.TrimSpace(req.Config)
	if req.Name == "" || req.Config == "" {
		jsonError(c, http.StatusBadRequest, "Both name and config are required.")
		return
	}
	if err := h.mcpConfigs.ValidateConfig(req.Config); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.mcpConfigs.Create(services.MCPConfigInput{
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

// UpdateMCPConfig 更新指定 MCP 配置并重新校验有效性。
func (h *HTTPController) UpdateMCPConfig(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name      string `json:"name"`
		Config    string `json:"config"`
		IsEnabled bool   `json:"isEnabled"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Config = strings.TrimSpace(req.Config)
	if req.Name == "" || req.Config == "" {
		jsonError(c, http.StatusBadRequest, "Both name and config are required.")
		return
	}
	if err := h.mcpConfigs.ValidateConfig(req.Config); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.mcpConfigs.Update(id, services.MCPConfigInput{
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
func (h *HTTPController) DeleteMCPConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.mcpConfigs.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
