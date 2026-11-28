package router

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"slimebot/internal/auth"
	"slimebot/internal/config"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	"slimebot/internal/server/controller"
	"slimebot/internal/server/ws"
	sessionsvc "slimebot/internal/services/session"
)

func TestSessionWorkbenchRoutes(t *testing.T) {
	ctx := context.Background()
	db := repositories.NewSQLiteDBTest(t, "session_workbench")
	repo := repositories.New(db)
	t.Cleanup(func() { _ = repo.Close() })
	sessions := sessionsvc.NewSessionService(repo)
	old, err := sessions.Create(ctx, "审查 / Branch")
	if err != nil {
		t.Fatal(err)
	}
	newer, err := sessions.Create(ctx, "New chat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: old.ID, Role: "user", Content: "First message"}); err != nil {
		t.Fatal(err)
	}
	answer, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: old.ID, Role: "assistant", Content: "Answer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ThinkingRecord{
		ID: "thinking", SessionID: old.ID, AssistantMessageID: &answer.ID, ThinkingID: "block", Content: "Private reasoning",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ToolCallRecord{
		ID: "tool", SessionID: old.ID, AssistantMessageID: &answer.ID, ToolCallID: "call",
		ToolName: "read_file", Status: "success", Output: "Tool result", ParamsJSON: `{"path":"README.md"}`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewTokenManager("session-workbench-test", 5)
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Generate("tester")
	if err != nil {
		t.Fatal(err)
	}
	h := &controller.HTTPController{}
	h.SetSessionWorkbenchService(sessions)
	handler := New(config.Config{}, tokens, h, ws.NewController(nil, nil), nil, RouterConfig{Headless: true})
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s status %d: %s", method, path, res.Code, res.Body)
		}
		return res
	}
	path := "/sessions/" + old.ID
	request(http.MethodPatch, path+"/pin", `{}`, http.StatusBadRequest)
	first := request(http.MethodPatch, path+"/pin", `{"pinned":true}`, http.StatusOK)
	second := request(http.MethodPatch, path+"/pin", `{"pinned":true}`, http.StatusOK)
	var a, b domain.Session
	if json.Unmarshal(first.Body.Bytes(), &a) != nil || json.Unmarshal(second.Body.Bytes(), &b) != nil ||
		!a.IsPinned || a.PinnedAt == nil || b.PinnedAt == nil || !a.PinnedAt.Equal(*b.PinnedAt) {
		t.Fatalf("pin is not idempotent: %s / %s", first.Body, second.Body)
	}
	list, err := sessions.List(ctx, 10, 0, "")
	if err != nil || len(list.Sessions) != 2 || list.Sessions[0].ID != old.ID || list.Sessions[1].ID != newer.ID {
		t.Fatalf("unexpected pinned ordering: %+v %v", list, err)
	}
	request(http.MethodPatch, path+"/pin", `{"pinned":false}`, http.StatusOK)
	res := request(http.MethodGet, path+"/export?format=json&includeTools=false", "", http.StatusOK)
	if body := res.Body.String(); strings.Contains(body, "Private reasoning") || strings.Contains(body, "Tool result") || !strings.Contains(body, "First message") {
		t.Fatalf("unexpected JSON export: %s", body)
	}
	res = request(http.MethodGet, path+"/export?includeThinking=true", "", http.StatusOK)
	if body := res.Body.String(); !strings.Contains(body, "<details><summary>Thinking") || !strings.Contains(body, "Tool result") || strings.Index(body, "First message") > strings.Index(body, "Answer") {
		t.Fatalf("unexpected Markdown export: %s", body)
	}
	_, params, err := mime.ParseMediaType(res.Header().Get("Content-Disposition"))
	if err != nil || !strings.HasPrefix(params["filename"], "slimebot-审查-Branch-") || !strings.HasSuffix(params["filename"], ".md") {
		t.Fatalf("filename header: %v, %v", params, err)
	}
	request(http.MethodGet, path+"/export?format=xml", "", http.StatusBadRequest)
}
