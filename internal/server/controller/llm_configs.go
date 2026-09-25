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
	req, ok := bindLLMConfigPayload(c)
	if !ok {
		return
	}
	item, err := h.llmConfigs.Create(c.Request().Context(), configsvc.LLMConfigCreateInput{
		Name:              req.Name,
		ProviderID:        req.ProviderID,
		Provider:          req.Provider,
		BaseURL:           req.BaseURL,
		APIKey:            req.APIKey,
		Model:             req.Model,
		ContextSize:       req.ContextSize,
		ContextSizeSource: req.ContextSizeSource,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateLLMConfig updates a model config using the same validation as create.
func (h *HTTPController) UpdateLLMConfig(c WebContext) {
	id := c.Param("id")
	req, ok := bindLLMConfigPayload(c)
	if !ok {
		return
	}
	if err := h.llmConfigs.Update(c.Request().Context(), id, configsvc.LLMConfigInput{
		Name:              req.Name,
		ProviderID:        req.ProviderID,
		Provider:          req.Provider,
		BaseURL:           req.BaseURL,
		APIKey:            req.APIKey,
		Model:             req.Model,
		ContextSize:       req.ContextSize,
		ContextSizeSource: req.ContextSizeSource,
	}); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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

type llmConfigPayload struct {
	Name              string `json:"name"`
	ProviderID        string `json:"providerId"`
	Provider          string `json:"provider"`
	BaseURL           string `json:"baseUrl"`
	APIKey            string `json:"apiKey"`
	Model             string `json:"model"`
	ContextSize       int    `json:"contextSize"`
	ContextSizeSource string `json:"contextSizeSource"`
}

func bindLLMConfigPayload(c WebContext) (llmConfigPayload, bool) {
	var req llmConfigPayload
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return req, false
	}
	trimSpaceFields(&req.Name, &req.ProviderID, &req.Provider, &req.BaseURL, &req.APIKey, &req.Model)
	if !allFieldsPresent(req.Name, req.Model) || (req.ProviderID == "" && !allFieldsPresent(req.BaseURL, req.APIKey)) {
		jsonError(c, http.StatusBadRequest, "name, model, and providerId are required.")
		return req, false
	}
	return req, true
}
