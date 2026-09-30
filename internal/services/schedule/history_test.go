package schedule

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
)

type historyRunner struct {
	run func(context.Context, domain.ScheduledTask) RunResult
}

func (r historyRunner) RunScheduledTask(ctx context.Context, task domain.ScheduledTask) RunResult {
	return r.run(ctx, task)
}

func TestRunHistoryLifecycleAndInterruptedRecovery(t *testing.T) {
	ctx := context.Background()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "run_history_lifecycle"))
	now := fixedNow()
	svc := NewService(repo, nil, Options{Now: func() time.Time { return now }})
	task, err := svc.Create(ctx, CreateInput{Name: "每日检查", Prompt: "检查状态", SessionID: "source", Schedule: ScheduleSpec{Kind: ScheduleKindOnce, RunAt: now}})
	if err != nil {
		t.Fatal(err)
	}
	var runningID string
	answer := strings.Repeat("状态正常", 100)
	svc.SetRunner(historyRunner{run: func(_ context.Context, _ domain.ScheduledTask) RunResult {
		page, err := svc.ListRuns(ctx, task.ID, "running", 20, 0)
		if err != nil || len(page.Runs) != 1 {
			t.Fatalf("running history: %+v, %v", page, err)
		}
		runningID = page.Runs[0].ID
		if page.Runs[0].SessionID != "" || page.Runs[0].FinishedAt != nil {
			t.Fatalf("premature outcome: %+v", page.Runs[0])
		}
		return RunResult{Success: true, Answer: "<!-- TOOL_CALL:hidden-id -->\n" + answer, SessionID: "actual-session", RequestID: "request", StartedAt: now, FinishedAt: now.Add(65 * time.Second)}
	}})
	if err := svc.RunDue(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListRuns(ctx, task.ID, "ok", 20, 0)
	if err != nil || len(page.Runs) != 1 || page.Runs[0].ID != runningID {
		t.Fatalf("duplicate or missing run: %+v, %v", page, err)
	}
	if len([]rune(page.Runs[0].Answer)) != 240 || page.Runs[0].TaskName != task.Name {
		t.Fatalf("history summary: %+v", page.Runs[0])
	}
	run, err := svc.GetRun(ctx, runningID)
	if err != nil || run.Answer != answer || run.SessionID != "actual-session" || run.RequestID != "request" {
		t.Fatalf("full result: %+v, %v", run, err)
	}
	if got := run.FinishedAt.Sub(run.StartedAt); got != 65*time.Second {
		t.Fatalf("duration = %v", got)
	}
	// Restart leaves completed executions untouched and marks only unfinished records.
	if err := repo.CreateScheduledTaskRun(ctx, &domain.ScheduledTaskRun{ID: "unfinished", TaskID: task.ID, Status: domain.ScheduledTaskRunStatusRunning, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RestoreRunningTasks(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := svc.GetRun(ctx, "unfinished")
	if err != nil || recovered.Status != "interrupted" || recovered.FinishedAt == nil || recovered.Error == "" {
		t.Fatalf("recovery: %+v, %v", recovered, err)
	}
	completed, _ := svc.GetRun(ctx, runningID)
	if completed.Status != "ok" {
		t.Fatalf("completed run altered: %+v", completed)
	}
}

func TestRunHistoryFiltersPaginationAndDeletion(t *testing.T) {
	ctx := context.Background()
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "run_history_pages"))
	svc := NewService(repo, nil, Options{Now: fixedNow})
	for _, id := range []string{"task-a", "task-b"} {
		if err := repo.CreateScheduledTask(ctx, &domain.ScheduledTask{ID: id, Name: id, SessionID: "source", Prompt: "check", Status: "scheduled", ScheduleKind: "interval"}); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{"a", "b", "c"} {
		if err := repo.CreateScheduledTaskRun(ctx, &domain.ScheduledTaskRun{ID: id, TaskID: "task-a", Status: "error", Error: "failed", StartedAt: fixedNow().Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateScheduledTaskRun(ctx, &domain.ScheduledTaskRun{ID: "d", TaskID: "task-b", Status: "ok", StartedAt: fixedNow()}); err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListRuns(ctx, "task-a", "error", 2, 0)
	if err != nil || !page.HasMore || len(page.Runs) != 2 || page.Runs[0].ID != "c" || page.Runs[1].ID != "b" {
		t.Fatalf("first page: %+v, %v", page, err)
	}
	last, err := svc.ListRuns(ctx, "task-a", "error", 2, 2)
	if err != nil || last.HasMore || len(last.Runs) != 1 || last.Runs[0].ID != "a" {
		t.Fatalf("last page: %+v, %v", last, err)
	}
	empty, err := svc.ListRuns(ctx, "task-a", "ok", 2, 0)
	if err != nil || empty.Runs == nil || len(empty.Runs) != 0 {
		t.Fatalf("empty: %+v, %v", empty, err)
	}
	if err := svc.Delete(ctx, "task-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetRun(ctx, "a"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("deleted run: %v", err)
	}
	page, _ = svc.ListRuns(ctx, "", "", 20, 0)
	if len(page.Runs) != 1 || page.Runs[0].ID != "d" {
		t.Fatalf("cascade: %+v", page)
	}
}

func TestFailedRunWithoutChatAndCancelledContextStillSaved(t *testing.T) {
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "run_history_cancel"))
	svc := NewService(repo, nil, Options{Now: fixedNow})
	task, err := svc.Create(context.Background(), CreateInput{Prompt: "check", SessionID: "source", Schedule: ScheduleSpec{Kind: ScheduleKindOnce, RunAt: fixedNow()}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.SetRunner(historyRunner{run: func(context.Context, domain.ScheduledTask) RunResult {
		cancel()
		return RunResult{Success: false, Error: "no model"}
	}})
	if err := svc.RunDue(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel result: %v", err)
	}
	page, err := svc.ListRuns(context.Background(), task.ID, "error", 20, 0)
	if err != nil || len(page.Runs) != 1 || page.Runs[0].SessionID != "" || page.Runs[0].Error != "no model" {
		t.Fatalf("failed run: %+v, %v", page, err)
	}
}
