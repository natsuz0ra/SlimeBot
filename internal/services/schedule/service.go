package schedule

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"

	"github.com/google/uuid"
)

type ScheduleKind string

const (
	ScheduleKindOnce     ScheduleKind = "once"
	ScheduleKindInterval ScheduleKind = "interval"
	ScheduleKindCron     ScheduleKind = "cron"
)

type ScheduleSpec struct {
	Kind            ScheduleKind `json:"kind"`
	RunAt           time.Time    `json:"runAt,omitempty"`
	IntervalMinutes int          `json:"intervalMinutes,omitempty"`
	CronExpr        string       `json:"cronExpr,omitempty"`
	Timezone        string       `json:"timezone,omitempty"`
}

type CreateInput struct {
	Name          string
	Prompt        string
	SessionID     string
	Schedule      ScheduleSpec
	ModelConfigID string
	ThinkingLevel string
	ApprovalMode  string
	MaxRuns       int
}

type UpdateInput struct {
	Name          *string
	Prompt        *string
	SessionID     *string
	Schedule      *ScheduleSpec
	ModelConfigID *string
	ThinkingLevel *string
	ApprovalMode  *string
	MaxRuns       *int
}

type RunResult struct {
	RequestID  string
	SessionID  string
	Success    bool
	Answer     string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}

type Options struct {
	Now func() time.Time
}

type Store interface {
	CreateScheduledTask(ctx context.Context, task *domain.ScheduledTask) error
	GetScheduledTask(ctx context.Context, id string) (*domain.ScheduledTask, error)
	ListScheduledTasks(ctx context.Context, includeInactive bool) ([]domain.ScheduledTask, error)
	ListDueScheduledTasks(ctx context.Context, now any) ([]domain.ScheduledTask, error)
	UpdateScheduledTask(ctx context.Context, id string, updates map[string]any) error
	DeleteScheduledTask(ctx context.Context, id string) error
	CreateScheduledTaskRun(ctx context.Context, run *domain.ScheduledTaskRun) error
	ListScheduledTaskRuns(ctx context.Context, taskID, status string, limit, offset int) ([]domain.ScheduledTaskRun, error)
	GetScheduledTaskRun(ctx context.Context, id string) (*domain.ScheduledTaskRun, error)
	UpdateScheduledTaskRun(ctx context.Context, id string, updates map[string]any) error
	InterruptScheduledTaskRuns(ctx context.Context, now time.Time) error
}

type Runner interface {
	RunScheduledTask(ctx context.Context, task domain.ScheduledTask) RunResult
}

type Service struct {
	store  Store
	runner Runner
	now    func() time.Time
}

func NewService(store Store, runner Runner, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, runner: runner, now: now}
}

func (s *Service) SetRunner(runner Runner) {
	if s == nil {
		return
	}
	s.runner = runner
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.ScheduledTask, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("schedule service is not initialized")
	}
	name := strings.TrimSpace(input.Name)
	prompt := strings.TrimSpace(input.Prompt)
	sessionID := strings.TrimSpace(input.SessionID)
	if name == "" {
		name = firstLine(prompt, "定时任务")
	}
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	spec, err := normalizeSpec(input.Schedule, s.now())
	if err != nil {
		return nil, err
	}
	nextRun, err := computeNextRun(spec, s.now(), nil)
	if err != nil {
		return nil, err
	}
	thinking := strings.TrimSpace(input.ThinkingLevel)
	if thinking == "" {
		thinking = "off"
	}
	approval := strings.TrimSpace(input.ApprovalMode)
	if approval == "" {
		approval = constants.ApprovalModeAuto
	}
	if !isValidApprovalMode(approval) {
		return nil, fmt.Errorf("invalid approval_mode: %s", approval)
	}
	task := &domain.ScheduledTask{
		ID:              uuid.NewString(),
		Name:            name,
		Prompt:          prompt,
		SessionID:       sessionID,
		ScheduleKind:    string(spec.Kind),
		RunAt:           timePtrOrNil(spec.RunAt),
		IntervalMinutes: spec.IntervalMinutes,
		CronExpr:        spec.CronExpr,
		Timezone:        spec.Timezone,
		ModelConfigID:   strings.TrimSpace(input.ModelConfigID),
		ThinkingLevel:   thinking,
		ApprovalMode:    approval,
		MaxRuns:         max(0, input.MaxRuns),
		Status:          domain.ScheduledTaskStatusScheduled,
		NextRunAt:       nextRun,
	}
	if err := s.store.CreateScheduledTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) List(ctx context.Context, includeInactive bool) ([]domain.ScheduledTask, error) {
	return s.store.ListScheduledTasks(ctx, includeInactive)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.ScheduledTask, error) {
	return s.store.GetScheduledTask(ctx, strings.TrimSpace(id))
}

type RunHistoryPage struct {
	Runs    []domain.ScheduledTaskRun `json:"runs"`
	HasMore bool                      `json:"hasMore"`
}

func (s *Service) ListRuns(ctx context.Context, taskID, status string, limit, offset int) (RunHistoryPage, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	runs, err := s.store.ListScheduledTaskRuns(ctx, strings.TrimSpace(taskID), status, limit+1, offset)
	if err != nil {
		return RunHistoryPage{}, err
	}
	more := len(runs) > limit
	if more {
		runs = runs[:limit]
	}
	if runs == nil {
		runs = []domain.ScheduledTaskRun{}
	}
	for i := range runs {
		answer := domain.StripContentMarkers(runs[i].Answer)
		// A bounded preview can end in the middle of an old timeline marker.
		if marker := strings.LastIndex(answer, "<!-- "); marker >= 0 && !strings.Contains(answer[marker:], "-->") {
			answer = answer[:marker]
		}
		runs[i].Answer = strings.TrimSpace(answer)
	}
	return RunHistoryPage{Runs: runs, HasMore: more}, nil
}

func (s *Service) GetRun(ctx context.Context, id string) (*domain.ScheduledTaskRun, error) {
	run, err := s.store.GetScheduledTaskRun(ctx, strings.TrimSpace(id))
	if err == nil {
		run.Answer = domain.StripContentMarkers(run.Answer)
	}
	return run, err
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.DeleteScheduledTask(ctx, strings.TrimSpace(id))
}

func (s *Service) Pause(ctx context.Context, id string) error {
	return s.store.UpdateScheduledTask(ctx, strings.TrimSpace(id), map[string]any{"status": domain.ScheduledTaskStatusPaused})
}

func (s *Service) Resume(ctx context.Context, id string) (*domain.ScheduledTask, error) {
	task, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	spec := specFromTask(*task)
	nextRun, err := computeNextRun(spec, s.now(), nil)
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateScheduledTask(ctx, task.ID, map[string]any{
		"status":      domain.ScheduledTaskStatusScheduled,
		"next_run_at": nextRun,
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, task.ID)
}

func (s *Service) Trigger(ctx context.Context, id string) (*domain.ScheduledTask, error) {
	now := s.now()
	if err := s.store.UpdateScheduledTask(ctx, strings.TrimSpace(id), map[string]any{
		"status":      domain.ScheduledTaskStatusScheduled,
		"next_run_at": &now,
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Update(ctx context.Context, id string, input UpdateInput) (*domain.ScheduledTask, error) {
	updates := map[string]any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		updates["name"] = name
	}
	if input.Prompt != nil {
		prompt := strings.TrimSpace(*input.Prompt)
		if prompt == "" {
			return nil, fmt.Errorf("prompt cannot be empty")
		}
		updates["prompt"] = prompt
	}
	if input.SessionID != nil {
		sessionID := strings.TrimSpace(*input.SessionID)
		if sessionID == "" {
			return nil, fmt.Errorf("session_id cannot be empty")
		}
		updates["session_id"] = sessionID
	}
	if input.ModelConfigID != nil {
		updates["model_config_id"] = strings.TrimSpace(*input.ModelConfigID)
	}
	if input.ThinkingLevel != nil {
		level := strings.TrimSpace(*input.ThinkingLevel)
		if level == "" {
			level = "off"
		}
		updates["thinking_level"] = level
	}
	if input.ApprovalMode != nil {
		mode := strings.TrimSpace(*input.ApprovalMode)
		if !isValidApprovalMode(mode) {
			return nil, fmt.Errorf("invalid approval_mode: %s", mode)
		}
		updates["approval_mode"] = mode
	}
	if input.MaxRuns != nil {
		updates["max_runs"] = max(0, *input.MaxRuns)
	}
	if input.Schedule != nil {
		spec, err := normalizeSpec(*input.Schedule, s.now())
		if err != nil {
			return nil, err
		}
		nextRun, err := computeNextRun(spec, s.now(), nil)
		if err != nil {
			return nil, err
		}
		updates["schedule_kind"] = string(spec.Kind)
		updates["run_at"] = timePtrOrNil(spec.RunAt)
		updates["interval_minutes"] = spec.IntervalMinutes
		updates["cron_expr"] = spec.CronExpr
		updates["timezone"] = spec.Timezone
		updates["next_run_at"] = nextRun
		updates["status"] = domain.ScheduledTaskStatusScheduled
	}
	if len(updates) == 0 {
		return s.Get(ctx, id)
	}
	if err := s.store.UpdateScheduledTask(ctx, strings.TrimSpace(id), updates); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Service) DueTasks(ctx context.Context) ([]domain.ScheduledTask, error) {
	now := s.now()
	candidates, err := s.store.ListDueScheduledTasks(ctx, now)
	if err != nil {
		return nil, err
	}
	due := make([]domain.ScheduledTask, 0, len(candidates))
	for _, task := range candidates {
		if task.NextRunAt == nil {
			continue
		}
		spec := specFromTask(task)
		if isRecurring(spec.Kind) && now.Sub(*task.NextRunAt) > graceForSpec(spec) {
			nextRun, err := computeNextRun(spec, now, &now)
			if err != nil {
				return nil, err
			}
			if err := s.store.UpdateScheduledTask(ctx, task.ID, map[string]any{"next_run_at": nextRun}); err != nil {
				return nil, err
			}
			continue
		}
		due = append(due, task)
	}
	return due, nil
}

func (s *Service) RestoreRunningTasks(ctx context.Context) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("schedule service is not initialized")
	}
	if err := s.store.InterruptScheduledTaskRuns(ctx, s.now()); err != nil {
		return err
	}
	tasks, err := s.store.ListScheduledTasks(ctx, true)
	if err != nil {
		return err
	}
	now := s.now()
	for _, task := range tasks {
		if task.Status != domain.ScheduledTaskStatusRunning {
			continue
		}
		nextRun := task.NextRunAt
		if nextRun == nil {
			spec := specFromTask(task)
			if !isRecurring(spec.Kind) && task.RunAt != nil {
				runAt := *task.RunAt
				nextRun = &runAt
			} else {
				computed, err := computeNextRun(spec, now, nil)
				if err != nil {
					return err
				}
				nextRun = computed
			}
		}
		if err := s.store.UpdateScheduledTask(ctx, task.ID, map[string]any{
			"status":      domain.ScheduledTaskStatusScheduled,
			"next_run_at": nextRun,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) MarkRunComplete(ctx context.Context, taskID string, result RunResult) error {
	return s.markRunComplete(ctx, taskID, "", result)
}

func (s *Service) markRunComplete(ctx context.Context, taskID, runID string, result RunResult) error {
	task, err := s.Get(ctx, taskID)
	if err != nil {
		return err
	}
	now := s.now()
	startedAt := result.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = now
	}
	status := domain.ScheduledTaskRunStatusOK
	if !result.Success {
		status = domain.ScheduledTaskRunStatusError
	}
	requestID := strings.TrimSpace(result.RequestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	runSessionID := strings.TrimSpace(result.SessionID)
	if runSessionID == "" && runID == "" {
		runSessionID = task.SessionID
	}
	run := &domain.ScheduledTaskRun{
		ID:         uuid.NewString(),
		TaskID:     task.ID,
		SessionID:  runSessionID,
		RequestID:  requestID,
		Status:     status,
		Answer:     domain.StripContentMarkers(result.Answer),
		Error:      result.Error,
		StartedAt:  startedAt,
		FinishedAt: &finishedAt,
	}
	if runID == "" {
		err = s.store.CreateScheduledTaskRun(ctx, run)
	} else {
		err = s.store.UpdateScheduledTaskRun(ctx, runID, map[string]any{"session_id": run.SessionID, "request_id": run.RequestID, "status": run.Status, "answer": run.Answer, "error": run.Error, "finished_at": run.FinishedAt})
	}
	if err != nil {
		return err
	}
	completedRuns := task.CompletedRuns + 1
	updates := map[string]any{
		"status":         domain.ScheduledTaskStatusScheduled,
		"last_run_at":    &finishedAt,
		"last_status":    status,
		"last_error":     "",
		"completed_runs": completedRuns,
	}
	if !result.Success {
		updates["status"] = domain.ScheduledTaskStatusError
		updates["last_error"] = result.Error
	}
	spec := specFromTask(*task)
	nextRun, err := computeNextRun(spec, now, &finishedAt)
	if err != nil {
		updates["status"] = domain.ScheduledTaskStatusError
		updates["last_error"] = err.Error()
		nextRun = nil
	}
	if task.MaxRuns > 0 && completedRuns >= task.MaxRuns {
		updates["status"] = domain.ScheduledTaskStatusCompleted
		nextRun = nil
	}
	if !result.Success && isRecurring(spec.Kind) {
		// Keep recurring failed tasks scheduled for the next computed run.
		updates["status"] = domain.ScheduledTaskStatusScheduled
	}
	if nextRun == nil {
		if !isRecurring(spec.Kind) && result.Success {
			updates["status"] = domain.ScheduledTaskStatusCompleted
		}
		updates["next_run_at"] = nil
	} else {
		updates["next_run_at"] = nextRun
	}
	return s.store.UpdateScheduledTask(ctx, task.ID, updates)
}

func (s *Service) PreAdvanceRecurring(ctx context.Context, task domain.ScheduledTask) error {
	spec := specFromTask(task)
	if !isRecurring(spec.Kind) {
		return nil
	}
	now := s.now()
	nextRun, err := computeNextRun(spec, now, &now)
	if err != nil {
		return err
	}
	return s.store.UpdateScheduledTask(ctx, task.ID, map[string]any{"next_run_at": nextRun, "status": domain.ScheduledTaskStatusRunning})
}

func (s *Service) RunDue(ctx context.Context) error {
	if s.runner == nil {
		return nil
	}
	due, err := s.DueTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range due {
		if err := s.PreAdvanceRecurring(ctx, task); err != nil {
			return err
		}
		if !isRecurring(ScheduleKind(task.ScheduleKind)) {
			if err := s.store.UpdateScheduledTask(ctx, task.ID, map[string]any{"status": domain.ScheduledTaskStatusRunning}); err != nil {
				return err
			}
		}
		run := &domain.ScheduledTaskRun{ID: uuid.NewString(), TaskID: task.ID, Status: domain.ScheduledTaskRunStatusRunning, StartedAt: s.now()}
		if err := s.store.CreateScheduledTaskRun(ctx, run); err != nil {
			return err
		}
		result := s.runner.RunScheduledTask(ctx, task)
		// Persist the outcome even when shutdown cancels the execution context.
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		err := s.markRunComplete(saveCtx, task.ID, run.ID, result)
		cancel()
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func normalizeSpec(spec ScheduleSpec, now time.Time) (ScheduleSpec, error) {
	spec.Kind = ScheduleKind(strings.TrimSpace(string(spec.Kind)))
	if spec.Timezone == "" {
		spec.Timezone = "Local"
	}
	switch spec.Kind {
	case ScheduleKindOnce:
		if spec.RunAt.IsZero() {
			return spec, fmt.Errorf("run_at is required for once schedule")
		}
		spec.IntervalMinutes = 0
		spec.CronExpr = ""
	case ScheduleKindInterval:
		if spec.IntervalMinutes <= 0 {
			return spec, fmt.Errorf("interval_minutes must be greater than 0")
		}
		spec.RunAt = time.Time{}
		spec.CronExpr = ""
	case ScheduleKindCron:
		spec.CronExpr = strings.TrimSpace(spec.CronExpr)
		if _, err := parseCronExpr(spec.CronExpr); err != nil {
			return spec, err
		}
		spec.RunAt = time.Time{}
		spec.IntervalMinutes = 0
	default:
		return spec, fmt.Errorf("schedule kind must be once, interval, or cron")
	}
	_ = now
	return spec, nil
}

func computeNextRun(spec ScheduleSpec, now time.Time, lastRunAt *time.Time) (*time.Time, error) {
	switch spec.Kind {
	case ScheduleKindOnce:
		if lastRunAt != nil {
			return nil, nil
		}
		runAt := spec.RunAt
		return &runAt, nil
	case ScheduleKindInterval:
		base := now
		if lastRunAt != nil && !lastRunAt.IsZero() {
			base = *lastRunAt
		}
		next := base.Add(time.Duration(spec.IntervalMinutes) * time.Minute)
		if !next.After(now) {
			missed := int(math.Floor(now.Sub(next).Minutes()/float64(spec.IntervalMinutes))) + 1
			next = next.Add(time.Duration(missed*spec.IntervalMinutes) * time.Minute)
		}
		return &next, nil
	case ScheduleKindCron:
		base := now
		if lastRunAt != nil && !lastRunAt.IsZero() {
			base = *lastRunAt
		}
		next, err := nextCronRun(spec.CronExpr, base, now)
		if err != nil {
			return nil, err
		}
		return &next, nil
	default:
		return nil, fmt.Errorf("unknown schedule kind: %s", spec.Kind)
	}
}

func specFromTask(task domain.ScheduledTask) ScheduleSpec {
	spec := ScheduleSpec{
		Kind:            ScheduleKind(task.ScheduleKind),
		IntervalMinutes: task.IntervalMinutes,
		CronExpr:        task.CronExpr,
		Timezone:        task.Timezone,
	}
	if task.RunAt != nil {
		spec.RunAt = *task.RunAt
	}
	return spec
}

func isRecurring(kind ScheduleKind) bool {
	return kind == ScheduleKindInterval || kind == ScheduleKindCron
}

func graceForSpec(spec ScheduleSpec) time.Duration {
	const minGrace = 2 * time.Minute
	const maxGrace = 2 * time.Hour
	if spec.Kind == ScheduleKindInterval && spec.IntervalMinutes > 0 {
		grace := time.Duration(spec.IntervalMinutes) * time.Minute / 2
		if grace < minGrace {
			return minGrace
		}
		if grace > maxGrace {
			return maxGrace
		}
		return grace
	}
	return minGrace
}

func firstLine(text, fallback string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return fallback
	}
	line := strings.TrimSpace(strings.Split(text, "\n")[0])
	if len([]rune(line)) > 50 {
		return string([]rune(line)[:50])
	}
	return line
}

func timePtrOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func isValidApprovalMode(mode string) bool {
	switch mode {
	case constants.ApprovalModeStandard, constants.ApprovalModeAutoReview, constants.ApprovalModeAuto:
		return true
	default:
		return false
	}
}

type cronField map[int]struct{}

type cronExpr struct {
	minute cronField
	hour   cronField
	day    cronField
	month  cronField
	week   cronField
}

func parseCronExpr(expr string) (cronExpr, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return cronExpr{}, fmt.Errorf("cron_expr must contain 5 fields")
	}
	minute, err := parseCronField(parts[0], 0, 59)
	if err != nil {
		return cronExpr{}, fmt.Errorf("invalid minute field: %w", err)
	}
	hour, err := parseCronField(parts[1], 0, 23)
	if err != nil {
		return cronExpr{}, fmt.Errorf("invalid hour field: %w", err)
	}
	day, err := parseCronField(parts[2], 1, 31)
	if err != nil {
		return cronExpr{}, fmt.Errorf("invalid day field: %w", err)
	}
	month, err := parseCronField(parts[3], 1, 12)
	if err != nil {
		return cronExpr{}, fmt.Errorf("invalid month field: %w", err)
	}
	week, err := parseCronField(parts[4], 0, 7)
	if err != nil {
		return cronExpr{}, fmt.Errorf("invalid weekday field: %w", err)
	}
	if _, ok := week[7]; ok {
		week[0] = struct{}{}
		delete(week, 7)
	}
	return cronExpr{minute: minute, hour: hour, day: day, month: month, week: week}, nil
}

func parseCronField(raw string, minValue, maxValue int) (cronField, error) {
	result := cronField{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty segment")
		}
		step := 1
		if strings.Contains(part, "/") {
			pieces := strings.Split(part, "/")
			if len(pieces) != 2 {
				return nil, fmt.Errorf("invalid step segment %q", part)
			}
			parsedStep, err := strconv.Atoi(pieces[1])
			if err != nil || parsedStep <= 0 {
				return nil, fmt.Errorf("invalid step %q", pieces[1])
			}
			step = parsedStep
			part = pieces[0]
		}
		start, end := minValue, maxValue
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			pieces := strings.Split(part, "-")
			if len(pieces) != 2 {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			var err error
			start, err = strconv.Atoi(pieces[0])
			if err != nil {
				return nil, err
			}
			end, err = strconv.Atoi(pieces[1])
			if err != nil {
				return nil, err
			}
		default:
			value, err := strconv.Atoi(part)
			if err != nil {
				return nil, err
			}
			start, end = value, value
		}
		if start < minValue || end > maxValue || start > end {
			return nil, fmt.Errorf("value out of range")
		}
		for value := start; value <= end; value += step {
			result[value] = struct{}{}
		}
	}
	return result, nil
}

func nextCronRun(expr string, base time.Time, now time.Time) (time.Time, error) {
	parsed, err := parseCronExpr(expr)
	if err != nil {
		return time.Time{}, err
	}
	candidate := base.Truncate(time.Minute).Add(time.Minute)
	limit := candidate.AddDate(5, 0, 0)
	for !candidate.After(limit) {
		if candidate.Before(now) || candidate.Equal(now) {
			candidate = candidate.Add(time.Minute)
			continue
		}
		if parsed.matches(candidate) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("failed to compute next cron run")
}

func (c cronExpr) matches(t time.Time) bool {
	_, okMinute := c.minute[t.Minute()]
	_, okHour := c.hour[t.Hour()]
	_, okDay := c.day[t.Day()]
	_, okMonth := c.month[int(t.Month())]
	_, okWeek := c.week[int(t.Weekday())]
	return okMinute && okHour && okDay && okMonth && okWeek
}

// MemoryStore is a small test store used by tool tests.
type MemoryStore struct {
	mu    sync.Mutex
	tasks map[string]domain.ScheduledTask
	runs  []domain.ScheduledTaskRun
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[string]domain.ScheduledTask)}
}

func (m *MemoryStore) CreateScheduledTask(_ context.Context, task *domain.ScheduledTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	m.tasks[task.ID] = *task
	return nil
}

func (m *MemoryStore) GetScheduledTask(_ context.Context, id string) (*domain.ScheduledTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("scheduled task %s not found", id)
	}
	return &task, nil
}

func (m *MemoryStore) ListScheduledTasks(_ context.Context, includeInactive bool) ([]domain.ScheduledTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tasks := make([]domain.ScheduledTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		if !includeInactive && (task.Status == domain.ScheduledTaskStatusPaused || task.Status == domain.ScheduledTaskStatusCompleted) {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func (m *MemoryStore) ListDueScheduledTasks(_ context.Context, now any) ([]domain.ScheduledTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	nowTime, _ := now.(time.Time)
	var tasks []domain.ScheduledTask
	for _, task := range m.tasks {
		if task.Status == domain.ScheduledTaskStatusScheduled && task.NextRunAt != nil && !task.NextRunAt.After(nowTime) {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *MemoryStore) UpdateScheduledTask(_ context.Context, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("scheduled task %s not found", id)
	}
	patchTask(&task, updates)
	m.tasks[id] = task
	return nil
}

func (m *MemoryStore) DeleteScheduledTask(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, id)
	return nil
}

func (m *MemoryStore) CreateScheduledTaskRun(_ context.Context, run *domain.ScheduledTaskRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	m.runs = append(m.runs, *run)
	return nil
}

func patchTask(task *domain.ScheduledTask, updates map[string]any) {
	for key, value := range updates {
		switch key {
		case "name":
			task.Name, _ = value.(string)
		case "prompt":
			task.Prompt, _ = value.(string)
		case "session_id":
			task.SessionID, _ = value.(string)
		case "schedule_kind":
			task.ScheduleKind, _ = value.(string)
		case "run_at":
			task.RunAt, _ = value.(*time.Time)
		case "interval_minutes":
			task.IntervalMinutes, _ = value.(int)
		case "cron_expr":
			task.CronExpr, _ = value.(string)
		case "timezone":
			task.Timezone, _ = value.(string)
		case "model_config_id":
			task.ModelConfigID, _ = value.(string)
		case "thinking_level":
			task.ThinkingLevel, _ = value.(string)
		case "approval_mode":
			task.ApprovalMode, _ = value.(string)
		case "max_runs":
			task.MaxRuns, _ = value.(int)
		case "completed_runs":
			task.CompletedRuns, _ = value.(int)
		case "status":
			task.Status, _ = value.(string)
		case "next_run_at":
			task.NextRunAt, _ = value.(*time.Time)
		case "last_run_at":
			task.LastRunAt, _ = value.(*time.Time)
		case "last_status":
			task.LastStatus, _ = value.(string)
		case "last_error":
			task.LastError, _ = value.(string)
		}
	}
	task.UpdatedAt = time.Now()
}

func (m *MemoryStore) ListScheduledTaskRuns(_ context.Context, taskID, status string, limit, offset int) ([]domain.ScheduledTaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runs := []domain.ScheduledTaskRun{}
	for _, run := range m.runs {
		task, exists := m.tasks[run.TaskID]
		if exists && (taskID == "" || run.TaskID == taskID) && (status == "" || run.Status == status) {
			run.TaskName = task.Name
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].StartedAt.Equal(runs[j].StartedAt) {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	if offset >= len(runs) {
		return []domain.ScheduledTaskRun{}, nil
	}
	return runs[offset:min(len(runs), offset+limit)], nil
}

func (m *MemoryStore) GetScheduledTaskRun(_ context.Context, id string) (*domain.ScheduledTaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.runs {
		if run.ID == id {
			run.TaskName = m.tasks[run.TaskID].Name
			return &run, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

func (m *MemoryStore) UpdateScheduledTaskRun(_ context.Context, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID != id {
			continue
		}
		run := &m.runs[i]
		if v, ok := updates["session_id"].(string); ok {
			run.SessionID = v
		}
		if v, ok := updates["request_id"].(string); ok {
			run.RequestID = v
		}
		if v, ok := updates["status"].(string); ok {
			run.Status = v
		}
		if v, ok := updates["answer"].(string); ok {
			run.Answer = v
		}
		if v, ok := updates["error"].(string); ok {
			run.Error = v
		}
		if v, ok := updates["finished_at"].(*time.Time); ok {
			run.FinishedAt = v
		}
		return nil
	}
	return apperrors.ErrNotFound
}

func (m *MemoryStore) InterruptScheduledTaskRuns(_ context.Context, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].Status == domain.ScheduledTaskRunStatusRunning {
			m.runs[i].Status = domain.ScheduledTaskRunStatusInterrupted
			m.runs[i].FinishedAt = &now
			m.runs[i].Error = "Execution was interrupted by an application restart."
		}
	}
	return nil
}
