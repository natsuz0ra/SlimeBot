package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/models"
)

// ListLLMConfigs 列出全部模型配置。
func (h *HTTPController) ListLLMConfigs(c *gin.Context) {
	items, err := h.repo.ListLLMConfigs()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateLLMConfig 创建模型配置并校验核心字段。
func (h *HTTPController) CreateLLMConfig(c *gin.Context) {
	var req models.LLMConfig
	if !bindJSONOrBadRequest(c, &req, "参数格式错误") {
		return
	}
	if req.Name == "" || req.BaseURL == "" || req.APIKey == "" || req.Model == "" {
		jsonError(c, http.StatusBadRequest, "name/baseUrl/apiKey/model 均必填")
		return
	}
	item, err := h.repo.CreateLLMConfig(req)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// DeleteLLMConfig 删除指定模型配置。
func (h *HTTPController) DeleteLLMConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.repo.DeleteLLMConfig(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
