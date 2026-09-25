package controller

import (
	"net/http"

	configsvc "slimebot/internal/services/config"
)

func (h *HTTPController) ListLLMProviders(c WebContext) {
	items, err := h.llmConfigs.ListProviders(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPController) CreateLLMProvider(c WebContext) {
	input, ok := bindLLMProvider(c)
	if !ok {
		return
	}
	item, err := h.llmConfigs.CreateProvider(c.Request().Context(), input)
	if err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *HTTPController) UpdateLLMProvider(c WebContext) {
	input, ok := bindLLMProvider(c)
	if !ok {
		return
	}
	if err := h.llmConfigs.UpdateProvider(c.Request().Context(), c.Param("id"), input); err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) DeleteLLMProvider(c WebContext) {
	if err := h.llmConfigs.DeleteProvider(c.Request().Context(), c.Param("id")); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) DiscoverLLMProviderModels(c WebContext) {
	items, err := h.llmConfigs.DiscoverModels(c.Request().Context(), c.Param("id"))
	if err != nil {
		jsonError(c, http.StatusBadGateway, err.Error())
		return
	}
	c.JSON(http.StatusOK, items)
}

func bindLLMProvider(c WebContext) (configsvc.LLMProviderInput, bool) {
	var input configsvc.LLMProviderInput
	if !bindJSONOrBadRequest(c, &input, "Invalid request payload format.") {
		return input, false
	}
	return input, true
}
