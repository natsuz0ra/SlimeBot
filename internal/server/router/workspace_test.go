package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"slimebot/internal/auth"
	"slimebot/internal/config"
	"slimebot/internal/repositories"
	"slimebot/internal/server/controller"
	"slimebot/internal/server/ws"
	sessionsvc "slimebot/internal/services/session"
)

func TestWorkspaceRoutesUseOnlyPersistedSessionDirectory(t *testing.T) {
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "workspace_routes"))
	defer repo.Close()
	root, other := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "file.txt"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	sessions := sessionsvc.NewSessionService(repo)
	session, err := sessions.Create(context.Background(), "Workspace chat", root)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager("workspace-route-test", 5)
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Generate("tester")
	if err != nil {
		t.Fatal(err)
	}
	h := controller.NewHTTPController(nil, sessions, nil, nil, nil, nil, nil, nil, nil, nil, tokens)
	router := New(config.Config{}, tokens, h, ws.NewController(nil, nil), nil, RouterConfig{Headless: true})
	base := "/api/sessions/" + session.ID + "/workspace/"
	req := httptest.NewRequest(http.MethodGet, base+"file?path=file.txt", nil)
	q := req.URL.Query()
	q.Set("cwd", other)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "inside") || strings.Contains(w.Body.String(), "outside") {
		t.Fatalf("root override was accepted: %d %s", w.Code, w.Body)
	}
	req = httptest.NewRequest(http.MethodGet, base+"file?path=%2e%2e%2ffile.txt", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("traversal = %d %s", w.Code, w.Body)
	}
}
