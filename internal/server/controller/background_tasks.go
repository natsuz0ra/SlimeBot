package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"
)

type backgroundTaskService interface {
	ListBackgroundTasks(context.Context, string, int) ([]domain.BackgroundTask, error)
	ActOnBackgroundTask(context.Context, string, string) (*domain.BackgroundTask, error)
}

func (h *HTTPController) SetBackgroundTaskService(service backgroundTaskService) {
	h.backgroundTasks = service
}

func (h *HTTPController) ListBackgroundTasks(c WebContext) {
	if h.backgroundTasks == nil {
		jsonError(c, http.StatusServiceUnavailable, "Background task service is unavailable.")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			jsonError(c, http.StatusBadRequest, "Invalid pagination.")
			return
		}
		limit = min(value, 200)
	}
	items, err := h.backgroundTasks.ListBackgroundTasks(c.Request().Context(), c.Param("id"), limit)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPController) ActOnBackgroundTask(c WebContext) {
	if h.backgroundTasks == nil {
		jsonError(c, http.StatusServiceUnavailable, "Background task service is unavailable.")
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if !bindJSONOrBadRequest(c, &input, "Invalid background task action.") {
		return
	}
	task, err := h.backgroundTasks.ActOnBackgroundTask(c.Request().Context(), c.Param("id"), input.Action)
	if errors.Is(err, apperrors.ErrNotFound) {
		jsonError(c, http.StatusNotFound, "Background task not found.")
		return
	}
	if errors.Is(err, apperrors.ErrInvalidInput) {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, chatsvc.ErrBackgroundTaskActive) {
		jsonError(c, http.StatusConflict, "The background task is still active.")
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, task)
}
