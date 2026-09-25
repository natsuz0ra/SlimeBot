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

func TestDesktopConfigIgnoresWebDataPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SLIMEBOT_HOME", home)
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "web.db"))
	t.Setenv("SKILLS_ROOT", filepath.Join(t.TempDir(), "web-skills"))
	t.Setenv("CHAT_UPLOAD_ROOT", filepath.Join(t.TempDir(), "web-uploads"))
	t.Setenv("HERMES_SKILLS_ROOTS", t.TempDir())
	t.Setenv("FRONTEND_ORIGIN", "https://example.org")
	cfg := desktopConfig("local-token")
	if cfg.DBPath != filepath.Join(home, "storage", "data.db") || cfg.SkillsRoot != filepath.Join(home, "skills") || cfg.ChatUploadRoot != filepath.Join(home, "storage", "chat_uploads") {
		t.Fatalf("desktop paths escaped the desktop home: %+v", cfg)
	}
	if len(cfg.HermesSkillsRoots) != 0 || cfg.Frontend != "" || cfg.JWTSecret != "local-token" {
		t.Fatalf("desktop config inherited web access settings: %+v", cfg)
	}
}
