package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	sandboxpolicy "slimebot/internal/sandbox"
	llm "slimebot/internal/services/llm"
	"slimebot/internal/tools"
)

var ErrBackgroundTaskActive = errors.New("background task is active")

func (s *ChatService) backgroundStore() (domain.BackgroundTaskStore, bool) {
	store, ok := s.store.(domain.BackgroundTaskStore)
	return store, ok && store != nil
}

func (s *ChatService) ListBackgroundTasks(ctx context.Context, sessionID string, limit int) ([]domain.BackgroundTask, error) {
	store, ok := s.backgroundStore()
	if !ok {
		return []domain.BackgroundTask{}, nil
	}
	items, err := store.ListBackgroundTasks(ctx, strings.TrimSpace(sessionID), limit)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []domain.BackgroundTask{}
	}
	for index := range items {
		items[index] = newBackgroundTask(items[index]).BackgroundTask
	}
	return items, nil
}

func (s *ChatService) GetBackgroundTask(ctx context.Context, id string) (*domain.BackgroundTask, error) {
	store, ok := s.backgroundStore()
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	task, err := store.GetBackgroundTask(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	*task = newBackgroundTask(*task).BackgroundTask
	return task, nil
}

func (s *ChatService) ActOnBackgroundTask(ctx context.Context, id, action string) (*domain.BackgroundTask, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "retry":
		return s.RetryBackgroundTask(ctx, id)
	case "continue":
		return s.ContinueBackgroundTask(ctx, id)
	case "stop":
		return s.StopBackgroundTask(ctx, id)
	default:
		return nil, fmt.Errorf("%w: action must be retry, continue, or stop", apperrors.ErrInvalidInput)
	}
}

func (s *ChatService) retryableBackgroundTask(ctx context.Context, id string) (*domain.BackgroundTask, error) {
	task, err := s.GetBackgroundTask(ctx, id)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case domain.BackgroundStatusInterrupted, domain.BackgroundStatusFailed, domain.BackgroundStatusStopped, domain.BackgroundStatusCompleted:
	default:
		return nil, ErrBackgroundTaskActive
	}
	if strings.TrimSpace(task.SessionID) == "" || strings.TrimSpace(task.Command) == "" || strings.TrimSpace(task.ProcessID) == "" {
		return nil, fmt.Errorf("%w: task has no session or command", apperrors.ErrInvalidInput)
	}
	return task, nil
}

func (s *ChatService) manualBackgroundOptions(ctx context.Context, task *domain.BackgroundTask) (backgroundOptions, error) {
	if _, err := s.store.GetSessionByID(ctx, task.SessionID); err != nil {
		return backgroundOptions{}, err
	}
	b := s.backgrounds()
	b.mu.Lock()
	epoch := b.epochs[task.SessionID]
	b.mu.Unlock()
	surface := task.Surface
	if surface == "" {
		surface = constants.ClientSurfaceFromContext(ctx)
	}
	return backgroundOptions{
		sessionID: task.SessionID,
		modelID:   task.ModelID,
		thinking:  task.Thinking,
		subagent:  task.Subagent,
		surface:   surface,
		approval:  s.backgroundApproval(ctx, task.Approval),
		plan:      task.PlanMode,
		epoch:     epoch,
		depth:     task.WakeDepth,
	}, nil
}

func (s *ChatService) RetryBackgroundTask(ctx context.Context, id string) (*domain.BackgroundTask, error) {
	source, err := s.retryableBackgroundTask(ctx, id)
	if err != nil {
		return nil, err
	}
	options, err := s.manualBackgroundOptions(ctx, source)
	if err != nil {
		return nil, err
	}
	tool, ok := tools.Get(constants.ExecToolName)
	if !ok {
		return nil, fmt.Errorf("exec tool is unavailable")
	}
	taskID := "proc-" + uuid.NewString()
	runCtx := constants.WithClientSurface(context.WithoutCancel(ctx), options.surface)
	runCtx = tools.WithProcessManager(runCtx, s.agent.getSessionProcessManager(source.SessionID))
	runCtx = sandboxpolicy.WithWorkingDirectory(runCtx, source.WorkingDirectory)
	runCtx = context.WithValue(runCtx, wakeDepthKey{}, 16)
	runCtx = context.WithValue(runCtx, backgroundTaskIDKey{}, taskID)
	runCtx = context.WithValue(runCtx, backgroundOriginTurnKey{}, source.ID)
	policy, err := s.resolveSandboxPolicy(runCtx, source.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	runCtx = sandboxpolicy.WithPolicy(runCtx, policy)
	state := &chatTurnState{session: &domain.Session{ID: source.SessionID}, modelConfig: llm.ModelRuntimeConfig{ConfigID: source.ModelID, ThinkingLevel: source.Thinking}}
	runCtx = tools.WithBackgroundObserver(runCtx, s.backgroundObserver(runCtx, state, source.Subagent, options.approval, source.PlanMode))
	result, runErr := tool.Execute(runCtx, "run", map[string]any{
		"command": source.Command, "shell": source.Shell,
		"working_directory": source.WorkingDirectory, "timeout_ms": fmt.Sprint(source.TimeoutMs),
		"background": true, "yield_time_ms": "0",
		"description": "User-requested retry of background task " + source.ID,
	})
	if runErr != nil || (result != nil && result.Error != "") {
		if runErr == nil {
			runErr = errors.New(result.Error)
		}
		s.backgrounds().task(source.SessionID, backgroundTask{domain.BackgroundTask{ID: taskID, TaskID: taskID, ProcessID: taskID, SessionID: source.SessionID, Command: source.Command, ModelID: source.ModelID, Thinking: source.Thinking, Subagent: source.Subagent, Surface: source.Surface, Approval: options.approval, PlanMode: source.PlanMode, AutoWake: source.AutoWake, WakeDepth: source.WakeDepth, Status: domain.BackgroundStatusFailed, Error: runErr.Error()}})
		return nil, runErr
	}
	return s.GetBackgroundTask(ctx, taskID)
}

func (s *ChatService) ContinueBackgroundTask(ctx context.Context, id string) (*domain.BackgroundTask, error) {
	source, err := s.retryableBackgroundTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(source.Output) == "" {
		return nil, fmt.Errorf("%w: task has no saved output", apperrors.ErrInvalidInput)
	}
	options, err := s.manualBackgroundOptions(ctx, source)
	if err != nil {
		return nil, err
	}
	b := s.backgrounds()
	event := tools.ProcessCompletion{ID: source.ProcessID, Command: source.Command, Output: source.Output, Status: "completed", ExitCode: source.ExitCode}
	b.mu.Lock()
	if b.ctx.Err() != nil {
		b.mu.Unlock()
		return nil, context.Canceled
	}
	b.queues[source.SessionID] = append(b.queues[source.SessionID], backgroundWake{options: options, event: event, taskID: source.ID})
	start := !b.workers[source.SessionID]
	if start {
		b.workers[source.SessionID] = true
		b.wg.Add(1)
	}
	b.mu.Unlock()
	b.task(source.SessionID, backgroundTask{domain.BackgroundTask{ID: source.ID, TaskID: source.ID, ProcessID: source.ProcessID, SessionID: source.SessionID, Status: domain.BackgroundStatusQueued, Output: source.Output, ExitCode: source.ExitCode}})
	if start {
		go b.work(source.SessionID)
	}
	return s.GetBackgroundTask(ctx, source.ID)
}

func (s *ChatService) StopBackgroundTask(ctx context.Context, id string) (*domain.BackgroundTask, error) {
	task, err := s.GetBackgroundTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if !backgroundTaskStatus(task.Status) {
		return task, nil
	}
	b := s.backgrounds()
	b.mu.Lock()
	b.epochs[task.SessionID]++
	cancel := b.activeCancel[task.SessionID]
	b.queues[task.SessionID] = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if task.Status == domain.BackgroundStatusRunning {
		manager := s.agent.getSessionProcessManager(task.SessionID)
		if _, ok := manager.Get(task.ProcessID); ok {
			manager.Stop(task.ProcessID)
			return s.GetBackgroundTask(ctx, task.ID)
		}
	}
	b.task(task.SessionID, backgroundTask{domain.BackgroundTask{ID: task.ID, TaskID: task.ID, ProcessID: task.ProcessID, SessionID: task.SessionID, Status: domain.BackgroundStatusStopped}})
	return s.GetBackgroundTask(ctx, task.ID)
}
