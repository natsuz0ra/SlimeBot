package controller

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	"slimebot/internal/services/schedule"
	sessionsvc "slimebot/internal/services/session"
	"strings"
	"testing"
	"time"
)

func TestRunHistoryValidatesQuery(t *testing.T) {
	h := NewHTTPController(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetScheduleService(schedule.NewService(schedule.NewMemoryStore(), nil, schedule.Options{}))
	for _, query := range []string{"status=unknown", "limit=0", "limit=-1", "limit=x", "offset=-1", "offset=1.5"} {
		w := httptest.NewRecorder()
		h.ListScheduledTaskRuns(NewChiContext(w, httptest.NewRequest(http.MethodGet, "/scheduled-task-runs?"+query, nil)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, %s", query, w.Code, w.Body.String())
		}
	}
	for _, query := range []string{"", "status=running", "status=interrupted", "limit=1000&offset=0"} {
		w := httptest.NewRecorder()
		h.ListScheduledTaskRuns(NewChiContext(w, httptest.NewRequest(http.MethodGet, "/scheduled-task-runs?"+query, nil)))
		if w.Code != http.StatusOK || w.Body.String() != "{\"runs\":[],\"hasMore\":false}\n" {
			t.Fatalf("%s: status %d, %s", query, w.Code, w.Body.String())
		}
	}
}

func TestTaskWorkspaceCreatesWithoutChatAndReadsExecution(t *testing.T) {
	ctx := context.Background()
	db := repositories.NewSQLiteDBTest(t, "task_workspace_api")
	repo := repositories.New(db)
	service := schedule.NewService(repo, nil, schedule.Options{})
	h := NewHTTPController(nil, sessionsvc.NewSessionService(repo), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetScheduleService(service)
	router := chi.NewRouter()
	router.Post("/tasks", func(w http.ResponseWriter, r *http.Request) { h.CreateScheduledTask(NewChiContext(w, r)) })
	router.Post("/tasks/{id}/actions", func(w http.ResponseWriter, r *http.Request) { h.ActOnScheduledTask(NewChiContext(w, r)) })
	router.Get("/runs/{id}/history", func(w http.ResponseWriter, r *http.Request) { h.GetScheduledTaskRunHistory(NewChiContext(w, r)) })
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	w := request("POST", "/tasks", `{"name":"日报","prompt":"生成日报","schedule":{"kind":"interval","intervalMinutes":60}}`)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var task domain.ScheduledTask
	if err := json.Unmarshal(w.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.SessionID != "" {
		t.Fatal("independent task has a chat")
	}
	var count int64
	if err := db.Model(&domain.Session{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("unexpected chats: %d %v", count, err)
	}
	for _, action := range []string{"pause", "resume", "run"} {
		w = request("POST", "/tasks/"+task.ID+"/actions", `{"action":"`+action+`"}`)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", action, w.Code, w.Body.String())
		}
	}
	if err := repo.UpdateScheduledTask(ctx, task.ID, map[string]any{"status": domain.ScheduledTaskStatusRunning}); err != nil {
		t.Fatal(err)
	}
	if w = request("POST", "/tasks/"+task.ID+"/actions", `{"action":"run"}`); w.Code != 409 {
		t.Fatalf("running task accepted: %d", w.Code)
	}
	execution, err := repo.CreateTaskRunContext(ctx, task.Name, "")
	if err != nil {
		t.Fatal(err)
	}
	message := domain.Message{ID: "instruction", SessionID: execution.ID, Role: "user", Content: "执行指令", Seq: 1, CreatedAt: time.Now()}
	if err := db.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.MarkRunComplete(ctx, task.ID, schedule.RunResult{SessionID: execution.ID, Success: true, Answer: "完成"}); err != nil {
		t.Fatal(err)
	}
	page, err := service.ListRuns(ctx, task.ID, "", 20, 0)
	if err != nil || len(page.Runs) != 1 {
		t.Fatalf("runs: %+v %v", page, err)
	}
	path := "/runs/" + page.Runs[0].ID + "/history"
	w = request("GET", path, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"messages":[`) || !strings.Contains(w.Body.String(), "执行指令") {
		t.Fatalf("history: %d %s", w.Code, w.Body.String())
	}
	for _, query := range []string{"?before=invalid", "?before=" + time.Now().UTC().Format(time.RFC3339), "?beforeSeq=-1"} {
		if w = request("GET", path+query, ""); w.Code != 400 {
			t.Fatalf("invalid cursor: %d %s", w.Code, w.Body.String())
		}
	}
	if w = request("GET", "/runs/missing/history", ""); w.Code != 404 {
		t.Fatalf("missing: %d", w.Code)
	}
	// Older failed records used the source chat as a fallback. Do not show that chat as execution steps.
	if err := repo.UpdateScheduledTask(ctx, task.ID, map[string]any{"session_id": execution.ID}); err != nil {
		t.Fatal(err)
	}
	w = request("GET", path, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "执行指令") {
		t.Fatalf("source chat exposed: %d %s", w.Code, w.Body.String())
	}
}
