package app

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"slimebot/internal/config"
)

func TestRunCLIHeadless_StaysReachableUntilClose(t *testing.T) {
	cfg := testConfig(t)

	app, err := NewHeadless(cfg)
	if err != nil {
		t.Fatalf("NewHeadless failed: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	closed := false
	defer func() {
		if !closed {
			app.Close(shutdownCtx)
		}
	}()

	if err := app.Start(shutdownCtx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the async server loop a brief moment.
	time.Sleep(150 * time.Millisecond)

	apiURL := "http://" + app.Addr()
	status, body, err := fetchHealth(apiURL, 2*time.Second)
	if err != nil {
		t.Fatalf("health check failed while app should be running: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: got %d body=%q", status, body)
	}

	app.Close(shutdownCtx)
	closed = true

	_, _, err = fetchHealth(apiURL, 500*time.Millisecond)
	if err == nil {
		t.Fatalf("expected health check to fail after Close")
	}
}

// testConfig returns a config pointing at a temp dir so tests don't clash with
// the real ~/.slimebot.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	tmp := t.TempDir()
	return config.Config{
		ServerPort:       "0",
		DBPath:           filepath.Join(tmp, "data.db"),
		SkillsRoot:       filepath.Join(tmp, "skills"),
		ChatUploadRoot:   filepath.Join(tmp, "uploads"),
		JWTSecret:        "test-secret",
		JWTExpireMinutes: 60,
	}
}

func fetchHealth(apiURL string, timeout time.Duration) (int, string, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(apiURL + "/health")
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(bodyBytes), nil
}
