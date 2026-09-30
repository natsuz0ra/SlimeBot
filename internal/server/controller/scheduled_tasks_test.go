package controller

import (
	"net/http"
	"net/http/httptest"
	"slimebot/internal/services/schedule"
	"testing"
)

func TestRunHistoryValidatesQuery(t *testing.T) {
	h := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetScheduleService(schedule.NewService(schedule.NewMemoryStore(), nil, schedule.Options{}))
	for _, query := range []string{"status=unknown", "limit=0", "limit=-1", "limit=x", "offset=-1", "offset=1.5"} {
		w := httptest.NewRecorder()
		h.ListScheduledTaskRuns(NewChiContext(w, httptest.NewRequest(http.MethodGet, "/scheduled-task-runs?"+query, nil)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, %s", query, w.Code, w.Body.String())
		}
	}
	for _, query := range []string{"", "status=running", "status=interrupted", "limit=1000&offset=0"} {
		w := httptest.NewRecorder()
		h.ListScheduledTaskRuns(NewChiContext(w, httptest.NewRequest(http.MethodGet, "/scheduled-task-runs?"+query, nil)))
		if w.Code != http.StatusOK || w.Body.String() != "{\"runs\":[],\"hasMore\":false}\n" {
			t.Fatalf("%s: status %d, %s", query, w.Code, w.Body.String())
		}
	}
}
