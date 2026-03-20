package controller

import (
	"net/http"

	configsvc "slimebot/backend/internal/services/config"
)

// ListLLMConfigs 鍒楀嚭鍏ㄩ儴妯″瀷閰嶇疆銆?

func (h *HTTPController) ListLLMConfigs(c WebContext) {
	items, err := h.llmConfigs.List()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateLLMConfig 鍒涘缓妯″瀷閰嶇疆骞舵牎楠屾牳蹇冨瓧娈点€?

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

// DeleteLLMConfig 鍒犻櫎鎸囧畾妯″瀷閰嶇疆銆?

func (h *HTTPController) DeleteLLMConfig(c WebContext) {
	id := c.Param("id")
	if err := h.llmConfigs.Delete(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
