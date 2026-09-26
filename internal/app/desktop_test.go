package app

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"slimebot/internal/config"
	"slimebot/internal/runtime"
)

func TestDesktopHostRequiresTokenAndHasNoLogin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SLIMEBOT_HOME", home)
	if err := runtime.EnsureAndLoadEnv(); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	cfg := config.Config{
		DBPath:           filepath.Join(home, "data.db"),
		SkillsRoot:       filepath.Join(home, "skills"),
		ChatUploadRoot:   filepath.Join(home, "uploads"),
		JWTSecret:        token,
		JWTExpireMinutes: 60,
	}
	instance, err := NewDesktop(cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer instance.Close(context.Background())
	if err := instance.Start(ctx); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{}
	base := "http://" + instance.Addr()
	check := func(method, endpoint, authorization string, want int) {
		t.Helper()
		req, err := http.NewRequest(method, base+endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s %s = %d, want %d: %s", method, endpoint, resp.StatusCode, want, body)
		}
	}
	check("GET", "/api/sessions", "", http.StatusUnauthorized)
	check("GET", "/api/sessions", "Bearer wrong", http.StatusUnauthorized)
	check("GET", "/api/sessions", "Bearer "+token, http.StatusOK)
	check("GET", "/api/settings", "Bearer "+token, http.StatusOK)
	check("POST", "/api/login", "", http.StatusUnauthorized)
	check("POST", "/api/login", "Bearer "+token, http.StatusNotFound)
	check("POST", "/api/update/apply", "Bearer "+token, http.StatusNotFound)
	wsURL := "ws://" + instance.Addr() + "/ws/chat"
	wsHeader := http.Header{"Origin": []string{base}, "Authorization": []string{"Bearer " + token}}
	connection, response, err := websocket.DefaultDialer.Dial(wsURL, wsHeader)
	if err != nil {
		t.Fatalf("desktop WebSocket failed: %v (status %v)", err, response)
	}
	_ = connection.Close()
	wsHeader.Set("Origin", "https://example.org")
	_, response, err = websocket.DefaultDialer.Dial(wsURL, wsHeader)
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign-origin WebSocket was accepted: %v (response %v)", err, response)
	}
}

func TestDesktopConfigUsesSharedDataPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SLIMEBOT_HOME", home)
	dbPath := filepath.Join(t.TempDir(), "web.db")
	skillsRoot := filepath.Join(t.TempDir(), "web-skills")
	uploadRoot := filepath.Join(t.TempDir(), "web-uploads")
	hermesRoot := t.TempDir()
	t.Setenv("DB_PATH", dbPath)
	t.Setenv("SKILLS_ROOT", skillsRoot)
	t.Setenv("CHAT_UPLOAD_ROOT", uploadRoot)
	t.Setenv("HERMES_SKILLS_ROOTS", hermesRoot)
	t.Setenv("FRONTEND_ORIGIN", "https://example.org")
	cfg := desktopConfig("local-token")
	if cfg.DBPath != dbPath || cfg.SkillsRoot != skillsRoot || cfg.ChatUploadRoot != uploadRoot {
		t.Fatalf("desktop does not use the shared data paths: %+v", cfg)
	}
	if len(cfg.HermesSkillsRoots) != 1 || cfg.HermesSkillsRoots[0] != hermesRoot || cfg.Frontend != "" || cfg.JWTSecret != "local-token" {
		t.Fatalf("desktop config did not preserve shared skills and local access settings: %+v", cfg)
	}
}
