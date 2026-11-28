package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	acpclient "slimebot/internal/acp/client"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"
	configsvc "slimebot/internal/services/config"

	"github.com/go-chi/chi/v5"
)

type acpControllerStore struct {
	mu    sync.Mutex
	items map[string]domain.ACPConfig
}

func newACPControllerStore() *acpControllerStore {
	return &acpControllerStore{items: make(map[string]domain.ACPConfig)}
}

func (s *acpControllerStore) ListACPConfigs(context.Context) ([]domain.ACPConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]domain.ACPConfig, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	return items, nil
}

func (s *acpControllerStore) GetACPConfig(_ context.Context, id string) (domain.ACPConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return domain.ACPConfig{}, apperrors.ErrNotFound
	}
	return item, nil
}

func (s *acpControllerStore) CreateACPConfig(_ context.Context, value *domain.ACPConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	s.items[value.ID] = *value
	return nil
}

func (s *acpControllerStore) UpdateACPConfig(_ context.Context, value *domain.ACPConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.UpdatedAt = time.Now().UTC()
	s.items[value.ID] = *value
	return nil
}

func (s *acpControllerStore) DeleteACPConfig(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return apperrors.ErrNotFound
	}
	delete(s.items, id)
	return nil
}

func (s *acpControllerStore) SetSessionACP(context.Context, string, *string, string) error {
	return nil
}

func newACPController(t *testing.T) (*HTTPController, *acpControllerStore) {
	t.Helper()
	store := newACPControllerStore()
	manager := acpclient.NewManager(store)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Close(ctx)
	})
	controller := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	controller.SetACPServices(configsvc.NewACPConfigService(store), manager, nil)
	return controller, store
}

func serveACP(controller *HTTPController, method, path string, handler func(WebContext), body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Method(method, path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(NewChiContext(w, r))
	}))
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, strings.Replace(path, "{id}", "cfg-1", 1), reader)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

func TestACPConfigControllerCRUD(t *testing.T) {
	controller, store := newACPController(t)

	create := serveACP(controller, http.MethodPost, "/api/acp-configs", controller.CreateACPConfig,
		`{"name":"Zed","command":"npx","args":["-y","agent"],"env":{"TOKEN":"top-secret"}}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "top-secret") {
		t.Fatalf("create response leaked env value: %s", create.Body.String())
	}
	var created domain.ACPConfig
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID == "" || created.TimeoutSec != 300 || !created.IsEnabled || len(created.Args) != 2 {
		t.Fatalf("unexpected created config: %+v", created)
	}
	if len(created.EnvKeys) != 1 || created.EnvKeys[0] != "TOKEN" {
		t.Fatalf("env keys = %v", created.EnvKeys)
	}
	if store.items[created.ID].EnvJSON == "" {
		t.Fatal("expected stored env json")
	}

	list := serveACP(controller, http.MethodGet, "/api/acp-configs", controller.ListACPConfigs, "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "top-secret") || !strings.Contains(list.Body.String(), `"envKeys":["TOKEN"]`) {
		t.Fatalf("list response = %d %s", list.Code, list.Body.String())
	}

	// Update to disabled, keeping the command and secrets.
	path := "/api/acp-configs/" + created.ID
	router := chi.NewRouter()
	router.Put("/api/acp-configs/{id}", func(w http.ResponseWriter, r *http.Request) {
		controller.UpdateACPConfig(NewChiContext(w, r))
	})
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"isEnabled":false}`))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", resp.Code, resp.Body.String())
	}
	var updated domain.ACPConfig
	if err := json.Unmarshal(resp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if updated.IsEnabled || updated.Name != "Zed" || !strings.Contains(store.items[created.ID].EnvJSON, "top-secret") {
		t.Fatalf("unexpected updated config: %+v", updated)
	}

	router.Delete("/api/acp-configs/{id}", func(w http.ResponseWriter, r *http.Request) {
		controller.DeleteACPConfig(NewChiContext(w, r))
	})
	req = httptest.NewRequest(http.MethodDelete, path, nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body=%s", resp.Code, resp.Body.String())
	}
	if len(store.items) != 0 {
		t.Fatalf("config not deleted: %+v", store.items)
	}
}

func TestACPConfigControllerTestConnectionReportsFailure(t *testing.T) {
	controller, _ := newACPController(t)
	create := serveACP(controller, http.MethodPost, "/api/acp-configs", controller.CreateACPConfig,
		`{"name":"Missing","command":"definitely-not-an-acp-binary-xyz"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var created domain.ACPConfig
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	router := chi.NewRouter()
	router.Post("/api/acp-configs/{id}/test", func(w http.ResponseWriter, r *http.Request) {
		controller.TestACPConnection(NewChiContext(w, r))
	})
	req := httptest.NewRequest(http.MethodPost, "/api/acp-configs/"+created.ID+"/test", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("test status = %d body=%s", resp.Code, resp.Body.String())
	}
	var result acpclient.TestResult
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.OK || result.Error == "" {
		t.Fatalf("expected failed connection result: %+v", result)
	}
}

func TestACPConfigControllerResetACPSession(t *testing.T) {
	controller, _ := newACPController(t)
	controller.SetACPServices(controller.acpConfigs, controller.acpClient, func(_ context.Context, id string) error {
		if id == "busy" {
			return chatsvc.ErrTurnBusy
		}
		return nil
	})
	router := chi.NewRouter()
	router.Post("/api/sessions/{id}/acp/reset", func(w http.ResponseWriter, r *http.Request) {
		controller.ResetACPSession(NewChiContext(w, r))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/session-1/acp/reset", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("reset status = %d body=%s", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/sessions/busy/acp/reset", nil)
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("busy reset status = %d body=%s", resp.Code, resp.Body.String())
	}
}
