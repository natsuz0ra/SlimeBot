package controller

import (
	"context"
	"net/http"
	"slimebot/internal/domain"
	usagesvc "slimebot/internal/services/usage"
	"strings"
	"time"
)

type usageService interface {
	Summary(context.Context, domain.UsageQuery) ([]domain.UsageSummary, error)
}

func (h *HTTPController) SetUsageService(service *usagesvc.Service) {
	h.usage = service
}

func parseOptionalUsageTime(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *HTTPController) GetUsageSummary(c WebContext) {
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
	since, err := parseOptionalUsageTime(c.Query("since"))
	if err != nil {
		jsonError(c, http.StatusBadRequest, "since must be a valid RFC3339 timestamp.")
		return
	}
	until, err := parseOptionalUsageTime(c.Query("until"))
	if err != nil {
		jsonError(c, http.StatusBadRequest, "until must be a valid RFC3339 timestamp.")
		return
	}
	if since != nil && until != nil && since.After(*until) {
		jsonError(c, http.StatusBadRequest, "since must not be after until.")
		return
	}
	items, err := h.usage.Summary(c.Request().Context(), domain.UsageQuery{
		GroupBy:       groupBy,
		RootSessionID: strings.TrimSpace(c.Query("rootSessionId")),
		Since:         since,
		Until:         until,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, map[string]any{"groupBy": groupBy, "items": items})
}
