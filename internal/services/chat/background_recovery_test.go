package chat

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llm "slimebot/internal/services/llm"
	"slimebot/internal/tools"
)

func TestBackgroundRecoveryDoesNotExecuteUntilManualRetry(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "后台恢复")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSetting(ctx, constants.SettingSandboxMode, "danger-full-access"); err != nil {
		t.Fatal(err)
	}
	service := NewChatService(repo, repo, llm.NewFactory(&backgroundCaptureProvider{}), nil, nil)
	command, shell := "printf retried", "sh"
	if runtime.GOOS == "windows" {
		command, shell = "Write-Output retried", "powershell"
	}
	original := &domain.BackgroundTask{ID: "proc-original", ProcessID: "proc-original", SessionID: session.ID, Command: command, Shell: shell, TimeoutMs: 5000, Status: domain.BackgroundStatusRunning, StartedAt: time.Now()}
	if err := repo.SaveBackgroundTask(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverBackgroundTasks(ctx); err != nil {
		t.Fatal(err)
	}
	interrupted, err := service.GetBackgroundTask(ctx, original.ID)
	if err != nil || interrupted.Status != domain.BackgroundStatusInterrupted {
		t.Fatalf("recovery: %+v %v", interrupted, err)
	}
	if running := service.agent.getSessionProcessManager(session.ID).List(); len(running) != 0 {
		t.Fatalf("recovery executed a process: %+v", running)
	}
	retried, err := service.RetryBackgroundTask(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID == original.ID {
		t.Fatal("retry must create a new execution")
	}
	deadline := time.Now().Add(5 * time.Second)
	for retried.Status == domain.BackgroundStatusRunning && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		retried, err = service.GetBackgroundTask(ctx, retried.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if retried.Status != domain.BackgroundStatusCompleted || !strings.Contains(retried.Output, "retried") || retried.AutoWake {
		t.Fatalf("retry result: %+v", retried)
	}
	originalAfterRetry, err := service.GetBackgroundTask(ctx, original.ID)
	if err != nil || originalAfterRetry.Status != domain.BackgroundStatusInterrupted {
		t.Fatalf("retry rewrote original execution: %+v %v", originalAfterRetry, err)
	}
}

func TestInterruptedBackgroundTaskContinuesFromSavedOutput(t *testing.T) {
	provider := &backgroundCaptureProvider{}
	repo := newTestRepo(t)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "继续后台任务")
	if err != nil {
		t.Fatal(err)
	}
	model, err := repo.CreateLLMConfig(ctx, domain.LLMConfig{Name: "fake", Provider: llm.ProviderOpenAI, BaseURL: "http://fake.invalid", APIKey: "synthetic", Model: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewChatService(repo, repo, llm.NewFactory(provider), nil, nil)
	t.Cleanup(func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		service.CloseBackground(shutdown)
	})
	task := &domain.BackgroundTask{ID: "proc-interrupted", ProcessID: "proc-interrupted", SessionID: session.ID, Command: "build", ModelID: model.ID, Status: domain.BackgroundStatusRunning, Output: "build finished", StartedAt: time.Now()}
	if err := repo.SaveBackgroundTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := service.RecoverBackgroundTasks(ctx); err != nil {
		t.Fatal(err)
	}
	continued, err := service.ContinueBackgroundTask(ctx, task.ID)
	if err != nil || continued.Status != domain.BackgroundStatusQueued {
		t.Fatalf("continue: %+v %v", continued, err)
	}
	waitBackgroundWorkers(t, service)
	continued, err = service.GetBackgroundTask(ctx, task.ID)
	if err != nil || continued.Status != domain.BackgroundStatusCompleted || continued.Output != "build finished" {
		t.Fatalf("continued result: %+v %v", continued, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.count != 1 || len(provider.messages) == 0 || provider.messages[len(provider.messages)-1].Content != "build finished" {
		t.Fatalf("saved output was not continued: calls=%d messages=%+v", provider.count, provider.messages)
	}
}

func TestBackgroundObserverPreservesDisabledAutomaticWake(t *testing.T) {
	provider := &backgroundCaptureProvider{}
	service, sessionID, modelID, ctx, end, _ := backgroundFixture(t, provider)
	defer end()
	session, err := service.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	model, err := service.ResolveModelRuntimeConfig(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	observer := service.backgroundObserver(ctx, &chatTurnState{session: session, modelConfig: model}, "", constants.ApprovalModeStandard, false)
	event := tools.ProcessCompletion{ID: "proc-no-wake", Command: "manual", Status: "completed", Output: "done"}
	observer.Started(event)
	observer.Completed(event)
	waitBackgroundWorkers(t, service)
	var task *domain.BackgroundTask
	items, err := service.ListBackgroundTasks(ctx, sessionID, 10)
	if err == nil && len(items) == 1 {
		task = &items[0]
	}
	if err != nil || task.Status != domain.BackgroundStatusCompleted || task.AutoWake {
		t.Fatalf("manual task: %+v %v", task, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.count != 0 {
		t.Fatal("manual task automatically resumed the conversation")
	}
}
