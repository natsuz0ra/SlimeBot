package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/models"
)

// ListMCPConfigs 列出全部 MCP 服务配置。
func (h *HTTPController) ListMCPConfigs(c *gin.Context) {
	items, err := h.repo.ListMCPConfigs()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateMCPConfig 创建 MCP 配置并执行配置内容校验。
func (h *HTTPController) CreateMCPConfig(c *gin.Context) {
	var req models.MCPConfig
	if !bindJSONOrBadRequest(c, &req, "参数格式错误") {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Config = strings.TrimSpace(req.Config)
	if req.Name == "" || req.Config == "" {
		jsonError(c, http.StatusBadRequest, "name/config 必填")
		return
	}
	if _, err := mcp.ParseAndValidateConfig(req.Config); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.repo.CreateMCPConfig(req)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateMCPConfig 更新指定 MCP 配置并重新校验有效性。
func (h *HTTPController) UpdateMCPConfig(c *gin.Context) {
	id := c.Param("id")
	var req models.MCPConfig
	if !bindJSONOrBadRequest(c, &req, "参数格式错误") {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Config = strings.TrimSpace(req.Config)
	if req.Name == "" || req.Config == "" {
		jsonError(c, http.StatusBadRequest, "name/config 必填")
		return
	}
	if _, err := mcp.ParseAndValidateConfig(req.Config); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateMCPConfig(id, req); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteMCPConfig 删除指定 MCP 配置。
func (h *HTTPController) DeleteMCPConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.DeleteMCPConfig(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
