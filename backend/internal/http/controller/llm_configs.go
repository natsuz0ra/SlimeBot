package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/services"
)

// ListLLMConfigs 列出全部模型配置。
func (h *HTTPController) ListLLMConfigs(c *gin.Context) {
	items, err := h.llmConfigs.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateLLMConfig 创建模型配置并校验核心字段。
func (h *HTTPController) CreateLLMConfig(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
		Model   string `json:"model"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.BaseURL) == "" || strings.TrimSpace(req.APIKey) == "" || strings.TrimSpace(req.Model) == "" {
		jsonError(c, http.StatusBadRequest, "name, baseUrl, apiKey, and model are all required.")
		return
	}
	item, err := h.llmConfigs.Create(services.LLMConfigCreateInput{
		Name:    req.Name,
		BaseURL: req.BaseURL,
		APIKey:  req.APIKey,
		Model:   req.Model,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// DeleteLLMConfig 删除指定模型配置。
func (h *HTTPController) DeleteLLMConfig(c *gin.Context) {
	id := c.Param("id")
	if err := h.llmConfigs.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
