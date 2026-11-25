package controller

import (
	"context"
	"net/http"
	"slimebot/internal/domain"
	usagesvc "slimebot/internal/services/usage"
	"strings"
)

type usageService interface {
	Summary(context.Context, domain.UsageQuery) ([]domain.UsageSummary, error)
}

func (h *HTTPController) SetUsageService(service *usagesvc.Service) {
	h.usage = service
}

func (h *HTTPController) GetUsageSummary(c WebContext) {
	if h.usage == nil {
		jsonError(c, http.StatusServiceUnavailable, "Usage service is unavailable.")
		return
	}
	groupBy := strings.ToLower(strings.TrimSpace(c.Query("groupBy")))
	if groupBy == "" {
		groupBy = domain.UsageGroupBySession
	}
	switch groupBy {
	case domain.UsageGroupBySession, domain.UsageGroupByModel, domain.UsageGroupByProject:
	default:
		jsonError(c, http.StatusBadRequest, "groupBy must be session, model, or project.")
		return
	}
	items, err := h.usage.Summary(c.Request().Context(), domain.UsageQuery{GroupBy: groupBy, RootSessionID: strings.TrimSpace(c.Query("rootSessionId"))})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, map[string]any{"groupBy": groupBy, "items": items})
}
