package controller

import (
	"net/http"

	configsvc "slimebot/internal/services/config"
)

// ListLLMConfigs 返回当前保存的全部模型配置。
func (h *HTTPController) ListLLMConfigs(c WebContext) {
	items, err := h.llmConfigs.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateLLMConfig 创建模型配置，并校验最基本的连接字段是否齐全。
func (h *HTTPController) CreateLLMConfig(c WebContext) {
	var req struct {
		Name    string `json:"name"`
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
		Model   string `json:"model"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	trimSpaceFields(&req.Name, &req.BaseURL, &req.APIKey, &req.Model)
	if !allFieldsPresent(req.Name, req.BaseURL, req.APIKey, req.Model) {
		jsonError(c, http.StatusBadRequest, "name, baseUrl, apiKey, and model are all required.")
		return
	}
	item, err := h.llmConfigs.Create(configsvc.LLMConfigCreateInput{
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
func (h *HTTPController) DeleteLLMConfig(c WebContext) {
	id := c.Param("id")
	if err := h.llmConfigs.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
