package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestSchedulerStopCancelsAndWaitsForRunToBeSaved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	runner := historyRunner{run: func(runCtx context.Context, _ domain.ScheduledTask) RunResult {
		close(started)
		<-runCtx.Done()
		close(cancelled)
		<-release
		return RunResult{Error: runCtx.Err().Error()}
	}}
	svc := NewService(NewMemoryStore(), runner, Options{Now: fixedNow})
	task, err := svc.Create(ctx, CreateInput{
		Prompt: "check", SessionID: "source",
		Schedule: ScheduleSpec{Kind: ScheduleKindOnce, RunAt: fixedNow()},
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler := NewScheduler(svc, time.Hour)
	scheduler.Start(ctx)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("scheduled run did not start")
	}
	scheduler.Start(ctx)

	expired, expire := context.WithCancel(ctx)
	expire()
	if err := scheduler.Stop(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop returned %v while the run was still active", err)
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("Stop did not cancel the scheduled run")
	}
	if err := scheduler.Wait(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait returned %v while the run was still active", err)
	}
	// Unblock the runner so the scheduler can save the result and exit.
	release <- struct{}{}
	if err := scheduler.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := svc.ListRuns(ctx, task.ID, "error", 20, 0)
	if err != nil || len(page.Runs) != 1 || page.Runs[0].Error != context.Canceled.Error() {
		t.Fatalf("run was not saved before Stop returned: %+v, %v", page, err)
	}
}
