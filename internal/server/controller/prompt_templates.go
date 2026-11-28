package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"slimebot/internal/apperrors"
	settingssvc "slimebot/internal/services/settings"
)

type promptTemplateService interface {
	PromptTemplates(context.Context) ([]settingssvc.PromptTemplate, error)
	UpdatePromptTemplates(context.Context, []settingssvc.PromptTemplate) ([]settingssvc.PromptTemplate, error)
}

func (h *HTTPController) SetPromptTemplateService(service promptTemplateService) {
	h.promptTemplates = service
}

func (h *HTTPController) GetPromptTemplates(c WebContext) {
	templates, err := h.promptTemplates.PromptTemplates(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, templates)
}

func (h *HTTPController) PutPromptTemplates(c WebContext) {
	var templates []settingssvc.PromptTemplate
	body := http.MaxBytesReader(c.Writer(), c.Request().Body, 12<<20)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&templates); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid prompt template payload.")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		jsonError(c, http.StatusBadRequest, "Expected a single JSON array.")
		return
	}
	saved, err := h.promptTemplates.UpdatePromptTemplates(c.Request().Context(), templates)
	if errors.Is(err, apperrors.ErrInvalidInput) {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, saved)
}
