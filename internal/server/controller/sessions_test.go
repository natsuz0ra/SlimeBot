package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSession_ReturnsBadRequestForMalformedJSON(t *testing.T) {
	controller := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewBufferString(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	controller.CreateSession(NewChiContext(resp, req))

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", resp.Code, resp.Body.String())
	}
}
