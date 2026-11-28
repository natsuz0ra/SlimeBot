package schedule

import (
	"context"
	"strings"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/repositories"
)

func TestNormalizeSpecValidatesTimezone(t *testing.T) {
	spec := ScheduleSpec{Kind: ScheduleKindCron, CronExpr: "0 9 * * *"}
	normalized, err := normalizeSpec(spec, fixedNow())
	if err != nil || normalized.Timezone != "Local" {
		t.Fatalf("default timezone = %q, error = %v", normalized.Timezone, err)
	}
	spec.Timezone = "Invalid/Location"
	if _, err := normalizeSpec(spec, fixedNow()); err == nil || !strings.Contains(err.Error(), spec.Timezone) {
		t.Fatalf("invalid timezone error = %v", err)
	}
}

func TestComputeNextCronRunInScheduleTimezone(t *testing.T) {
	utc := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2026, month, day, hour, minute, 0, 0, time.UTC)
	}
	for _, tt := range []struct {
		name, zone, expr string
		now, want        time.Time
	}{
		{"UTC", "UTC", "0 9 * * *", utc(time.January, 12, 0, 0), utc(time.January, 12, 9, 0)},
		{"Shanghai", "Asia/Shanghai", "0 9 * * *", utc(time.January, 12, 0, 0), utc(time.January, 12, 1, 0)},
		{"New York", "America/New_York", "0 9 * * *", utc(time.January, 12, 0, 0), utc(time.January, 12, 14, 0)},
		{"spring gap", "America/New_York", "30 2 * * *", utc(time.March, 8, 6, 0), utc(time.March, 9, 6, 30)},
		{"first fall overlap", "America/New_York", "30 1 * * *", utc(time.November, 1, 5, 0), utc(time.November, 1, 5, 30)},
		{"second fall overlap", "America/New_York", "30 1 * * *", utc(time.November, 1, 5, 30), utc(time.November, 1, 6, 30)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := ScheduleSpec{Kind: ScheduleKindCron, CronExpr: tt.expr, Timezone: tt.zone}
			next, err := computeNextRun(spec, tt.now.In(time.FixedZone("test", 3*60*60)), nil)
			if err != nil || next == nil || !next.Equal(tt.want) || next.Location() != time.UTC {
				t.Fatalf("next run = %v, error = %v, want %v", next, err, tt.want)
			}
		})
	}
}

func TestCronDueAtSameInstantAcrossTimezones(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.January, 12, 0, 0, 0, 0, time.FixedZone("server", -5*60*60))
	repo := repositories.New(repositories.NewSQLiteDBTest(t, "cron_due_timezone"))
	svc := NewService(repo, nil, Options{Now: func() time.Time { return now }})
	task, err := svc.Create(ctx, CreateInput{
		Prompt: "check", SessionID: "source",
		Schedule: ScheduleSpec{Kind: ScheduleKindCron, CronExpr: "0 14 * * *", Timezone: "Asia/Shanghai"},
	})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Minute)
	if due, err := svc.DueTasks(ctx); err != nil || len(due) != 0 {
		t.Fatalf("tasks due before the scheduled instant: %v, %v", due, err)
	}
	now = now.Add(time.Minute)
	if due, err := svc.DueTasks(ctx); err != nil || len(due) != 1 || due[0].ID != task.ID {
		t.Fatalf("tasks due at the scheduled instant: %v, %v", due, err)
	}
}

func TestServiceCreateTaskComputesNextRunAndDefaults(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "schedule_create")
	repo := repositories.New(db)
	svc := NewService(repo, nil, Options{Now: fixedNow})

	session, err := repo.CreateSession(context.Background(), "定时任务")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	task, err := svc.Create(context.Background(), CreateInput{
		Name:      "每日摘要",
		Prompt:    "每天总结项目状态",
		SessionID: session.ID,
		Schedule:  ScheduleSpec{Kind: ScheduleKindInterval, IntervalMinutes: 30},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if task.ID == "" {
		t.Fatal("task ID should be generated")
	}
	if task.SessionID != session.ID {
		t.Fatalf("session ID = %q, want %q", task.SessionID, session.ID)
	}
	if task.Status != domain.ScheduledTaskStatusScheduled {
		t.Fatalf("status = %q, want scheduled", task.Status)
	}
	if task.ApprovalMode != constants.ApprovalModeAuto {
		t.Fatalf("approval mode = %q, want auto", task.ApprovalMode)
	}
	want := fixedNow().Add(30 * time.Minute)
	if !task.NextRunAt.Equal(want) {
		t.Fatalf("next run = %s, want %s", task.NextRunAt, want)
	}
}

func TestServiceDueTasksFastForwardsStaleRecurringTasks(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "schedule_due")
	repo := repositories.New(db)
	now := fixedNow()
	svc := NewService(repo, nil, Options{Now: func() time.Time { return now }})

	session, err := repo.CreateSession(context.Background(), "定时任务")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	task, err := svc.Create(context.Background(), CreateInput{
		Name:      "轮询",
		Prompt:    "检查一次状态",
		SessionID: session.ID,
		Schedule:  ScheduleSpec{Kind: ScheduleKindInterval, IntervalMinutes: 10},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	stale := now.Add(-45 * time.Minute)
	if err := repo.UpdateScheduledTask(context.Background(), task.ID, map[string]any{"next_run_at": stale}); err != nil {
		t.Fatalf("UpdateScheduledTask failed: %v", err)
	}

	due, err := svc.DueTasks(context.Background())
	if err != nil {
		t.Fatalf("DueTasks failed: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("due tasks = %d, want 0 after stale fast-forward", len(due))
	}
	got, err := repo.GetScheduledTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetScheduledTask failed: %v", err)
	}
	if !got.NextRunAt.After(now) {
		t.Fatalf("next run should be fast-forwarded after now, got %s", got.NextRunAt)
	}
}

func TestServiceRestoreRunningRecurringTask(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "schedule_restore_recurring")
	repo := repositories.New(db)
	now := fixedNow()
	svc := NewService(repo, nil, Options{Now: func() time.Time { return now }})

	session, err := repo.CreateSession(context.Background(), "定时任务")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	task, err := svc.Create(context.Background(), CreateInput{
		Name:      "轮询",
		Prompt:    "检查一次状态",
		SessionID: session.ID,
		Schedule:  ScheduleSpec{Kind: ScheduleKindInterval, IntervalMinutes: 10},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	nextRun := now.Add(-time.Minute)
	if err := repo.UpdateScheduledTask(context.Background(), task.ID, map[string]any{
		"status":      domain.ScheduledTaskStatusRunning,
		"next_run_at": &nextRun,
	}); err != nil {
		t.Fatalf("UpdateScheduledTask failed: %v", err)
	}

	if err := svc.RestoreRunningTasks(context.Background()); err != nil {
		t.Fatalf("RestoreRunningTasks failed: %v", err)
	}

	got, err := repo.GetScheduledTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetScheduledTask failed: %v", err)
	}
	if got.Status != domain.ScheduledTaskStatusScheduled {
		t.Fatalf("status = %q, want scheduled", got.Status)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(nextRun) {
		t.Fatalf("next run = %s, want preserved %s", got.NextRunAt, nextRun)
	}
}

func TestServiceRestoreRunningOneShotTaskIsDueAgain(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "schedule_restore_oneshot")
	repo := repositories.New(db)
	now := fixedNow()
	svc := NewService(repo, nil, Options{Now: func() time.Time { return now }})

	session, err := repo.CreateSession(context.Background(), "定时任务")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	task, err := svc.Create(context.Background(), CreateInput{
		Name:      "一次提醒",
		Prompt:    "提醒我喝水",
		SessionID: session.ID,
		Schedule:  ScheduleSpec{Kind: ScheduleKindOnce, RunAt: now.Add(-time.Minute)},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.UpdateScheduledTask(context.Background(), task.ID, map[string]any{
		"status": domain.ScheduledTaskStatusRunning,
	}); err != nil {
		t.Fatalf("UpdateScheduledTask failed: %v", err)
	}

	if err := svc.RestoreRunningTasks(context.Background()); err != nil {
		t.Fatalf("RestoreRunningTasks failed: %v", err)
	}
	due, err := svc.DueTasks(context.Background())
	if err != nil {
		t.Fatalf("DueTasks failed: %v", err)
	}
	if len(due) != 1 || due[0].ID != task.ID {
		t.Fatalf("due tasks = %+v, want restored one-shot task", due)
	}
}

func TestServiceMarkRunCompletesOneShotTask(t *testing.T) {
	db := repositories.NewSQLiteDBTest(t, "schedule_oneshot")
	repo := repositories.New(db)
	now := fixedNow()
	svc := NewService(repo, nil, Options{Now: func() time.Time { return now }})

	session, err := repo.CreateSession(context.Background(), "定时任务")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	task, err := svc.Create(context.Background(), CreateInput{
		Name:      "一次提醒",
		Prompt:    "提醒我喝水",
		SessionID: session.ID,
		Schedule:  ScheduleSpec{Kind: ScheduleKindOnce, RunAt: now.Add(time.Minute)},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.MarkRunComplete(context.Background(), task.ID, RunResult{
		Success: true,
		Answer:  "完成",
	}); err != nil {
		t.Fatalf("MarkRunComplete failed: %v", err)
	}
	got, err := repo.GetScheduledTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetScheduledTask failed: %v", err)
	}
	if got.Status != domain.ScheduledTaskStatusCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if got.NextRunAt != nil {
		t.Fatalf("next run = %s, want nil", got.NextRunAt)
	}
}

func TestServiceMarkRunCompleteRecordsActualRunSession(t *testing.T) {
	store := NewMemoryStore()
	svc := NewService(store, nil, Options{Now: fixedNow})
	runAt := fixedNow().Add(time.Minute)
	task := &domain.ScheduledTask{
		ID:           "task-actual-session",
		Name:         "每日摘要",
		Prompt:       "总结项目状态",
		SessionID:    "source-session",
		ScheduleKind: string(ScheduleKindOnce),
		RunAt:        &runAt,
		Status:       domain.ScheduledTaskStatusRunning,
	}
	if err := store.CreateScheduledTask(context.Background(), task); err != nil {
		t.Fatalf("CreateScheduledTask failed: %v", err)
	}

	if err := svc.MarkRunComplete(context.Background(), task.ID, RunResult{
		SessionID: "run-session",
		RequestID: "request-1",
		Success:   true,
		Answer:    "完成",
	}); err != nil {
		t.Fatalf("MarkRunComplete failed: %v", err)
	}

	if len(store.runs) != 1 {
		t.Fatalf("run records = %d, want 1", len(store.runs))
	}
	if store.runs[0].SessionID != "run-session" {
		t.Fatalf("run session ID = %q, want run-session", store.runs[0].SessionID)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 23, 9, 0, 0, 0, time.Local)
}
