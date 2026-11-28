package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestScheduledTaskClaimIsAtomicAndExclusive(t *testing.T) {
	ctx := context.Background()
	db := NewSQLiteDBTest(t, "scheduled_claim_exclusive")
	repo := New(db)
	now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
	next := now.Add(10 * time.Minute)
	task := &domain.ScheduledTask{
		ID: "claim-task", Name: "claim", Prompt: "run", SessionID: "source",
		ScheduleKind: string(domain.ScheduleKindInterval), IntervalMinutes: 10,
		Timezone: "UTC", Status: domain.ScheduledTaskStatusScheduled, NextRunAt: &now,
	}
	if err := repo.CreateScheduledTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	first := &domain.ScheduledTaskRun{ID: "run-1", TaskID: task.ID, Status: domain.ScheduledTaskRunStatusRunning, StartedAt: now}
	claimed, err := repo.ClaimScheduledTaskRun(ctx, task.ID, now, &next, first)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	second := &domain.ScheduledTaskRun{ID: "run-2", TaskID: task.ID, Status: domain.ScheduledTaskRunStatusRunning, StartedAt: now}
	claimed, err = repo.ClaimScheduledTaskRun(ctx, task.ID, now, &next, second)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = %v, %v", claimed, err)
	}
	got, err := repo.GetScheduledTask(ctx, task.ID)
	if err != nil || got.Status != domain.ScheduledTaskStatusRunning || got.NextRunAt == nil || !got.NextRunAt.Equal(next) {
		t.Fatalf("claimed task = %+v, %v", got, err)
	}
	var runs int64
	if err := db.Model(&domain.ScheduledTaskRun{}).Where("task_id = ?", task.ID).Count(&runs).Error; err != nil || runs != 1 {
		t.Fatalf("claimed runs = %d, %v", runs, err)
	}
}

func TestScheduledTaskCompletionIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	db := NewSQLiteDBTest(t, "scheduled_complete_idempotent")
	repo := New(db)
	now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
	task := &domain.ScheduledTask{
		ID: "complete-task", Name: "complete", Prompt: "run", SessionID: "source",
		ScheduleKind: string(domain.ScheduleKindOnce), RunAt: &now,
		Timezone: "UTC", Status: domain.ScheduledTaskStatusScheduled, NextRunAt: &now,
	}
	if err := repo.CreateScheduledTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimScheduledTaskRun(ctx, task.ID, now, task.NextRunAt, &domain.ScheduledTaskRun{ID: "run-1", StartedAt: now})
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	finished := now.Add(time.Second)
	completion := &domain.ScheduledTaskRun{ID: "run-1", TaskID: task.ID, RequestID: "request-1", Status: domain.ScheduledTaskRunStatusOK, Answer: "done", StartedAt: now, FinishedAt: &finished}
	updates := map[string]any{
		"status": domain.ScheduledTaskStatusCompleted, "completed_runs": 1,
		"last_run_at": &finished, "last_status": domain.ScheduledTaskRunStatusOK, "next_run_at": nil,
	}
	completed, err := repo.CompleteScheduledTaskRun(ctx, task.ID, "run-1", completion, updates)
	if err != nil || !completed {
		t.Fatalf("first completion = %v, %v", completed, err)
	}
	duplicate := *completion
	duplicate.Answer = "must not replace terminal result"
	completed, err = repo.CompleteScheduledTaskRun(ctx, task.ID, "run-1", &duplicate, map[string]any{"completed_runs": 2})
	if err != nil || completed {
		t.Fatalf("duplicate completion = %v, %v", completed, err)
	}
	gotTask, err := repo.GetScheduledTask(ctx, task.ID)
	if err != nil || gotTask.CompletedRuns != 1 || gotTask.Status != domain.ScheduledTaskStatusCompleted {
		t.Fatalf("completed task = %+v, %v", gotTask, err)
	}
	gotRun, err := repo.GetScheduledTaskRun(ctx, "run-1")
	if err != nil || gotRun.Status != domain.ScheduledTaskRunStatusOK || gotRun.Answer != "done" || gotRun.FinishedAt == nil {
		t.Fatalf("completed run = %+v, %v", gotRun, err)
	}
}

func TestScheduledTaskTransactionsRollBack(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
	newTask := func(repo *Repository, id string) {
		t.Helper()
		if err := repo.CreateScheduledTask(ctx, &domain.ScheduledTask{
			ID: id, Name: id, Prompt: "run", SessionID: "source", ScheduleKind: string(domain.ScheduleKindOnce),
			RunAt: &now, Timezone: "UTC", Status: domain.ScheduledTaskStatusScheduled, NextRunAt: &now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("claim", func(t *testing.T) {
		db := NewSQLiteDBTest(t, "scheduled_claim_rollback")
		repo := New(db)
		newTask(repo, "rollback-claim")
		if err := db.Exec(`CREATE TRIGGER fail_running_run BEFORE INSERT ON scheduled_task_runs
			WHEN NEW.status = 'running' BEGIN SELECT RAISE(ABORT, 'injected claim failure'); END`).Error; err != nil {
			t.Fatal(err)
		}
		claimed, err := repo.ClaimScheduledTaskRun(ctx, "rollback-claim", now, &now, &domain.ScheduledTaskRun{ID: "run", StartedAt: now})
		if err == nil || claimed || !strings.Contains(err.Error(), "injected claim failure") {
			t.Fatalf("claim = %v, %v", claimed, err)
		}
		task, getErr := repo.GetScheduledTask(ctx, "rollback-claim")
		if getErr != nil || task.Status != domain.ScheduledTaskStatusScheduled {
			t.Fatalf("task after rollback = %+v, %v", task, getErr)
		}
		var count int64
		if countErr := db.Model(&domain.ScheduledTaskRun{}).Count(&count).Error; countErr != nil || count != 0 {
			t.Fatalf("runs after rollback = %d, %v", count, countErr)
		}
	})

	t.Run("completion", func(t *testing.T) {
		db := NewSQLiteDBTest(t, "scheduled_complete_rollback")
		repo := New(db)
		newTask(repo, "rollback-complete")
		claimed, err := repo.ClaimScheduledTaskRun(ctx, "rollback-complete", now, &now, &domain.ScheduledTaskRun{ID: "run", StartedAt: now})
		if err != nil || !claimed {
			t.Fatalf("claim = %v, %v", claimed, err)
		}
		if err := db.Exec(`CREATE TRIGGER fail_task_completion BEFORE UPDATE OF completed_runs ON scheduled_tasks
			BEGIN SELECT RAISE(ABORT, 'injected completion failure'); END`).Error; err != nil {
			t.Fatal(err)
		}
		finished := now.Add(time.Second)
		completed, err := repo.CompleteScheduledTaskRun(ctx, "rollback-complete", "run", &domain.ScheduledTaskRun{
			Status: domain.ScheduledTaskRunStatusOK, StartedAt: now, FinishedAt: &finished,
		}, map[string]any{"completed_runs": 1, "status": domain.ScheduledTaskStatusCompleted})
		if err == nil || completed || !strings.Contains(err.Error(), "injected completion failure") {
			t.Fatalf("completion = %v, %v", completed, err)
		}
		var run domain.ScheduledTaskRun
		if getErr := db.Where("id = ?", "run").Take(&run).Error; getErr != nil || run.Status != domain.ScheduledTaskRunStatusRunning || run.FinishedAt != nil {
			t.Fatalf("run after rollback = %+v, %v", run, getErr)
		}
		task, getErr := repo.GetScheduledTask(ctx, "rollback-complete")
		if getErr != nil || task.CompletedRuns != 0 || task.Status != domain.ScheduledTaskStatusRunning {
			t.Fatalf("task after rollback = %+v, %v", task, getErr)
		}
	})
}
