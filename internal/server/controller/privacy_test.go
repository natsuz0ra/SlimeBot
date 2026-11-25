package controller

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestPrivacyMappingSettingsPreserveOmittedOptions(t *testing.T) {
	store := &privacyTestStore{}
	dir := t.TempDir()
	update := func(s *privacysvc.Service, body string) *httptest.ResponseRecorder {
		h := &HTTPController{privacy: s}
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/privacy", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		h.UpdatePrivacy(NewChiContext(w, r))
		return w
	}
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", "")
	missing := privacysvc.NewService(store, dir)
	defer missing.Close()
	if got := update(missing, `{"mappingPersistence":true}`); got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "SLIMEBOT_PRIVACY_MAPPING_KEY") {
		t.Fatalf("missing deployment key response: %d %s", got.Code, got.Body.String())
	}
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{11}, 32)))
	enabled := privacysvc.NewService(store, dir)
	defer enabled.Close()
	for _, body := range []string{`{"mappingPersistence":true,"mappingTtlMinutes":30}`, `{"enabled":true}`} {
		if got := update(enabled, body); got.Code != http.StatusNoContent {
			t.Fatalf("settings update: %d %s", got.Code, got.Body.String())
		}
	}
	config, err := enabled.GetConfig(context.Background())
	if err != nil || !config.Enabled || !config.MappingPersistence || config.MappingTTLMinutes != 30 {
		t.Fatalf("legacy request replaced lifecycle options: %+v %v", config, err)
	}
}
