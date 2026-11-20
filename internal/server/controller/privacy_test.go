package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	privacysvc "slimebot/internal/services/privacy"
)

type privacyTestStore struct{ raw string }

func (s *privacyTestStore) GetSetting(context.Context, string) (string, error) { return s.raw, nil }
func (s *privacyTestStore) SetSetting(_ context.Context, _, raw string) error {
	s.raw = raw
	return nil
}

func TestPrivacyAPIValidationAndPreview(t *testing.T) {
	s := privacysvc.NewService(&privacyTestStore{}, t.TempDir())
	defer s.Close()
	h := &HTTPController{privacy: s}
	for _, tc := range []struct {
		body    string
		handler func(WebContext)
		want    int
	}{
		{`{"enabled":true,"rules":[{"kind":"CUSTOM","pattern":"("}]}`, h.UpdatePrivacy, http.StatusBadRequest},
		{`{"enabled":true,"words":["Project-Rain"]}`, h.UpdatePrivacy, http.StatusNoContent},
		{`{"text":"Project-Rain needs DB_PASSWORD=SyntheticSecret123"}`, h.PreviewPrivacy, http.StatusOK},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/privacy", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		tc.handler(NewChiContext(w, r))
		if w.Code != tc.want {
			t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
		}
		if tc.want == http.StatusOK && (strings.Contains(w.Body.String(), "Project-Rain") || strings.Contains(w.Body.String(), "SyntheticSecret123")) {
			t.Fatal("preview response leaked a masked value")
		}
	}
	if snapshot, err := s.Snapshot(context.Background()); err != nil || len(snapshot.Records) != 0 {
		t.Fatal("preview created request records")
	}
}
