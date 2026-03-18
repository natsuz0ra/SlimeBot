package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/repositories"
	"slimebot/backend/internal/testutil"
)

func TestCreateSession_ReturnsBadRequestForMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := repositories.New(testutil.NewSQLiteDB(t, "http_sessions_test"))
	controller := NewHTTPController(repo, nil, nil, nil, nil)

	engine := gin.New()
	engine.POST("/sessions", controller.CreateSession)

	req := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewBufferString(`{"name":`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	engine.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", resp.Code, resp.Body.String())
	}
}
