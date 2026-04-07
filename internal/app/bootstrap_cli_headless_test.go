package app

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestRunCLIHeadless_StaysReachableUntilClose(t *testing.T) {
	app, err := RunCLIHeadless()
	if err != nil {
		t.Fatalf("RunCLIHeadless failed: %v", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	closed := false
	defer func() {
		if !closed {
			app.Close(shutdownCtx)
		}
	}()

	// Give the async server loop a brief moment; old behavior shuts down here due to deferred stopSignals.
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
