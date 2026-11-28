package tools

import (
	"context"
	"maps"

	"slimebot/internal/domain"
	"slimebot/internal/sandbox"
)

type MemoryStore interface {
	Add(context.Context, domain.MemoryTarget, string) (domain.MemoryTargetState, error)
	Replace(context.Context, domain.MemoryTarget, string, string) (domain.MemoryTargetState, error)
	Remove(context.Context, domain.MemoryTarget, string) (domain.MemoryTargetState, error)
	Snapshot(context.Context) (domain.MemorySnapshot, error)
}

type ScheduleService interface {
	Create(context.Context, domain.ScheduledTaskCreateInput) (*domain.ScheduledTask, error)
	List(context.Context, bool) ([]domain.ScheduledTask, error)
	Get(context.Context, string) (*domain.ScheduledTask, error)
	Pause(context.Context, string) error
	Resume(context.Context, string) (*domain.ScheduledTask, error)
	Delete(context.Context, string) error
	Trigger(context.Context, string) (*domain.ScheduledTask, error)
	Update(context.Context, string, domain.ScheduledTaskUpdateInput) (*domain.ScheduledTask, error)
}

type SkillRuntime interface {
	ListSkills() ([]domain.Skill, error)
	ActivateSkill(string, map[string]struct{}) (string, bool, error)
}

// SubagentRunRequest is the tool-layer input passed to the chat agent runner.
type SubagentRunRequest struct {
	Title   string
	Task    string
	Context string
	Params  map[string]any
}

type SubagentRunner interface {
	RunSubagent(context.Context, SubagentRunRequest) (*ExecuteResult, error)
}

// TurnToolRuntime carries the dependencies of a tool execution. Value copies
// share the concurrency-safe session stores. Each child owns its skill map.
type TurnToolRuntime struct {
	SessionID       string
	SandboxPolicy   *sandbox.Policy
	ReadFileState   *ReadFileState
	TodoState       *TodoState
	ProcessManager  *ProcessManager
	SkillRuntime    SkillRuntime
	ActivatedSkills map[string]struct{}
	Memory          MemoryStore
	Schedule        ScheduleService
	SubagentRunner  SubagentRunner
}

func (r TurnToolRuntime) WithSubagentRunner(runner SubagentRunner) TurnToolRuntime {
	r.SubagentRunner = runner
	return r
}

// WithActivatedSkills snapshots the activation set for an independent execution.
func (r TurnToolRuntime) WithActivatedSkills(activated map[string]struct{}) TurnToolRuntime {
	r.ActivatedSkills = make(map[string]struct{}, len(activated))
	maps.Copy(r.ActivatedSkills, activated)
	return r
}

type turnToolRuntimeKey struct{}

// WithTurnToolRuntime bridges the typed runtime to the registered Tool API.
func WithTurnToolRuntime(ctx context.Context, runtime TurnToolRuntime) context.Context {
	if runtime.SandboxPolicy != nil {
		ctx = sandbox.WithPolicy(ctx, runtime.SandboxPolicy)
	}
	return context.WithValue(ctx, turnToolRuntimeKey{}, runtime)
}

func turnToolRuntimeFromContext(ctx context.Context) TurnToolRuntime {
	if ctx == nil {
		return TurnToolRuntime{}
	}
	runtime, _ := ctx.Value(turnToolRuntimeKey{}).(TurnToolRuntime)
	return runtime
}

func WithSkillRuntime(ctx context.Context, skill SkillRuntime) context.Context {
	if skill == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.SkillRuntime = skill
	return WithTurnToolRuntime(ctx, runtime)
}

func skillRuntimeFromContext(ctx context.Context) (SkillRuntime, bool) {
	skill := turnToolRuntimeFromContext(ctx).SkillRuntime
	return skill, skill != nil
}

func WithActivatedSkills(ctx context.Context, activated map[string]struct{}) context.Context {
	if activated == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.ActivatedSkills = activated
	return WithTurnToolRuntime(ctx, runtime)
}

func activatedSkillsFromContext(ctx context.Context) map[string]struct{} {
	if activated := turnToolRuntimeFromContext(ctx).ActivatedSkills; activated != nil {
		return activated
	}
	return make(map[string]struct{})
}

func WithSubagentRunner(ctx context.Context, runner SubagentRunner) context.Context {
	if runner == nil {
		return ctx
	}
	return WithTurnToolRuntime(ctx, turnToolRuntimeFromContext(ctx).WithSubagentRunner(runner))
}

func subagentRunnerFromContext(ctx context.Context) (SubagentRunner, bool) {
	runner := turnToolRuntimeFromContext(ctx).SubagentRunner
	return runner, runner != nil
}

func WithReadFileState(ctx context.Context, state *ReadFileState) context.Context {
	if state == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.ReadFileState = state
	return WithTurnToolRuntime(ctx, runtime)
}

func readFileStateFromContext(ctx context.Context) *ReadFileState {
	return turnToolRuntimeFromContext(ctx).ReadFileState
}

func WithTodoState(ctx context.Context, state *TodoState) context.Context {
	if state == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.TodoState = state
	return WithTurnToolRuntime(ctx, runtime)
}

func todoStateFromContext(ctx context.Context) *TodoState {
	if state := turnToolRuntimeFromContext(ctx).TodoState; state != nil {
		return state
	}
	return defaultTodoState
}

func WithProcessManager(ctx context.Context, manager *ProcessManager) context.Context {
	if manager == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.ProcessManager = manager
	return WithTurnToolRuntime(ctx, runtime)
}

func processManagerFromContext(ctx context.Context) *ProcessManager {
	if manager := turnToolRuntimeFromContext(ctx).ProcessManager; manager != nil {
		return manager
	}
	return defaultProcessManager
}

func WithMemoryService(ctx context.Context, service MemoryStore) context.Context {
	if service == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.Memory = service
	return WithTurnToolRuntime(ctx, runtime)
}

func memoryServiceFromContext(ctx context.Context) (MemoryStore, bool) {
	service := turnToolRuntimeFromContext(ctx).Memory
	return service, service != nil
}

func WithScheduleService(ctx context.Context, service ScheduleService) context.Context {
	if service == nil {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.Schedule = service
	return WithTurnToolRuntime(ctx, runtime)
}

func scheduleServiceFromContext(ctx context.Context) (ScheduleService, bool) {
	service := turnToolRuntimeFromContext(ctx).Schedule
	return service, service != nil
}

func WithCurrentSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	runtime := turnToolRuntimeFromContext(ctx)
	runtime.SessionID = sessionID
	return WithTurnToolRuntime(ctx, runtime)
}

func currentSessionIDFromContext(ctx context.Context) string {
	return turnToolRuntimeFromContext(ctx).SessionID
}
