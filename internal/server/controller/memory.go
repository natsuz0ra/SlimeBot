package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	memorysvc "slimebot/internal/services/memory"
)

func (h *HTTPController) GetMemory(c WebContext) {
	snapshot, err := h.memory.Snapshot(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *HTTPController) ClearMemory(c WebContext) {
	targetText := strings.TrimSpace(c.Param("target"))
	if strings.EqualFold(targetText, "all") {
		if _, err := h.memory.Clear(c.Request().Context(), memorysvc.TargetMemory); err != nil {
			jsonInternalError(c, err)
			return
		}
		if _, err := h.memory.Clear(c.Request().Context(), memorysvc.TargetUser); err != nil {
			jsonInternalError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
		return
	}
	target, err := memorysvc.NormalizeTarget(targetText)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "target must be memory, user, or all.")
		return
	}
	if _, err := h.memory.Clear(c.Request().Context(), target); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) DeleteMemoryEntry(c WebContext) {
	target, err := memorysvc.NormalizeTarget(c.Param("target"))
	if err != nil {
		jsonError(c, http.StatusBadRequest, "target must be memory or user.")
		return
	}
	index, err := strconv.Atoi(strings.TrimSpace(c.Param("index")))
	if err != nil || index < 0 {
		jsonError(c, http.StatusBadRequest, "index must be a non-negative integer.")
		return
	}
	if _, err := h.memory.RemoveIndex(c.Request().Context(), target, index); err != nil {
		if errors.Is(err, memorysvc.ErrEntryNotFound) {
			jsonError(c, http.StatusNotFound, "Memory entry not found.")
			return
		}
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
