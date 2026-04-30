package controller

import (
	"net/http"

	configsvc "slimebot/internal/services/config"
)

// ListLLMConfigs returns all saved LLM model configs.
func (h *HTTPController) ListLLMConfigs(c WebContext) {
	items, err := h.llmConfigs.List(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

// CreateLLMConfig creates a model config and validates required connection fields.
func (h *HTTPController) CreateLLMConfig(c WebContext) {
	var req struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
		BaseURL  string `json:"baseUrl"`
		APIKey   string `json:"apiKey"`
		Model    string `json:"model"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	trimSpaceFields(&req.Name, &req.Provider, &req.BaseURL, &req.APIKey, &req.Model)
	if !allFieldsPresent(req.Name, req.BaseURL, req.APIKey, req.Model) {
		jsonError(c, http.StatusBadRequest, "name, baseUrl, apiKey, and model are all required.")
		return
	}
	item, err := h.llmConfigs.Create(c.Request().Context(), configsvc.LLMConfigCreateInput{
		Name:     req.Name,
		Provider: req.Provider,
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		Model:    req.Model,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// DeleteLLMConfig removes a model config by id.
func (h *HTTPController) DeleteLLMConfig(c WebContext) {
	id := c.Param("id")
	if err := h.llmConfigs.Delete(c.Request().Context(), id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
