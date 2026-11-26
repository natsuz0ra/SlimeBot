package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slimebot/internal/domain"
	"testing"
)

type usageServiceStub struct {
	lastQuery domain.UsageQuery
	items     []domain.UsageSummary
}

func (s *usageServiceStub) Summary(_ context.Context, query domain.UsageQuery) ([]domain.UsageSummary, error) {
	s.lastQuery = query
	return s.items, nil
}

func TestGetUsageSummaryParsesAndValidatesTimeRange(t *testing.T) {
	stub := &usageServiceStub{
		items: []domain.UsageSummary{
			{GroupKey: "s1", RootSessionID: "s1", SessionName: "Demo", SessionExists: true, OutputTokens: 42},
		},
	}
	handler := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.usage = stub

	// Valid since & until
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/summary?groupBy=session&since=2026-11-20T00:00:00Z&until=2026-11-26T00:00:00.000Z", nil)
	handler.GetUsageSummary(NewChiContext(rec, req))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.lastQuery.Since == nil || stub.lastQuery.Until == nil {
		t.Fatalf("expected parsed since/until, got %+v", stub.lastQuery)
	}
	var payload struct {
		GroupBy string                `json:"groupBy"`
		Items   []domain.UsageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil || len(payload.Items) != 1 || payload.Items[0].SessionName != "Demo" {
		t.Fatalf("unexpected payload: %s %v", rec.Body.String(), err)
	}

	// Invalid since
	for _, rawURL := range []string{
		"/api/usage/summary?since=not-a-date",
		"/api/usage/summary?until=2026-11-26",
		"/api/usage/summary?since=2026-11-27T00:00:00Z&until=2026-11-26T00:00:00Z",
	} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, rawURL, nil)
		handler.GetUsageSummary(NewChiContext(rec, req))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", rawURL, rec.Code, rec.Body.String())
		}
	}
}
