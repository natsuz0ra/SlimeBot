package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
)

func (h *HTTPController) ListScheduledTasks(c WebContext) {
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	tasks, err := h.schedule.List(c.Request().Context(), true)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if tasks == nil {
		tasks = []domain.ScheduledTask{}
	}
	c.JSON(http.StatusOK, tasks)
}

func (h *HTTPController) ListScheduledTaskRuns(c WebContext) {
	status := strings.TrimSpace(c.Query("status"))
	switch status {
	case "", domain.ScheduledTaskRunStatusRunning, domain.ScheduledTaskRunStatusOK, domain.ScheduledTaskRunStatusError, domain.ScheduledTaskRunStatusInterrupted:
	default:
		jsonError(c, http.StatusBadRequest, "Invalid run status.")
		return
	}
	limit, offset := 20, 0
	for _, field := range []string{"limit", "offset"} {
		raw := strings.TrimSpace(c.Query(field))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || (field == "limit" && value == 0) {
			jsonError(c, http.StatusBadRequest, "Invalid pagination.")
			return
		}
		if field == "limit" {
			limit = min(value, 50)
		} else {
			offset = value
		}
	}
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	page, err := h.schedule.ListRuns(c.Request().Context(), c.Query("taskId"), status, limit, offset)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *HTTPController) GetScheduledTaskRun(c WebContext) {
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	run, err := h.schedule.GetRun(c.Request().Context(), c.Param("id"))
	if errors.Is(err, apperrors.ErrNotFound) {
		jsonError(c, http.StatusNotFound, "Run record not found.")
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}
