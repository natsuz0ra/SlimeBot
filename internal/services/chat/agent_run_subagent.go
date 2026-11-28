package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	"slimebot/internal/services/subagent"
	teamsvc "slimebot/internal/services/team"
	"slimebot/internal/tools"

	"github.com/google/uuid"
)

const maxSubagentTitleRunes = 80

// SubagentExecutor runs one run_subagent call using the selected execution engine.
type SubagentExecutor interface {
	ExecuteSubagent(context.Context, subagentExecutionRequest) (*tools.ExecuteResult, error)
}

type subagentExecutionRequest struct {
	parentModel     llmsvc.ModelRuntimeConfig
	sessionID       string
	mcpConfigs      []domain.MCPConfig
	activatedSkills map[string]struct{}
	callbacks       AgentCallbacks
	opts            AgentLoopOptions
	toolCall        llmsvc.ToolCallInfo
	invocation      resolvedToolInvocation
	params          map[string]any
	preamble        string
	reservedMember  *domain.TeamMemberRun
	reservationErr  error
}

// agentSubagentRunner adapts the registered tool to the chat execution port.
type agentSubagentRunner struct {
	executor SubagentExecutor
	request  subagentExecutionRequest
}

func (r agentSubagentRunner) RunSubagent(ctx context.Context, input tools.SubagentRunRequest) (*tools.ExecuteResult, error) {
	r.request.params = input.Params
	if r.request.params == nil {
		r.request.params = map[string]any{"title": input.Title, "task": input.Task, "context": input.Context}
	}
	return r.executor.ExecuteSubagent(ctx, r.request)
}

func (a *AgentService) subagentExecutor(loop *cooperativeLoop) SubagentExecutor {
	if loop != nil {
		return cooperativeSubagentExecutor{host: a.subagentHost, loop: loop}
	}
	return directSubagentExecutor{agent: a}
}

type subagentInput struct {
	title   string
	task    string
	context string
}

func (r subagentExecutionRequest) admit(host SubagentHost, loop *cooperativeLoop) (subagentInput, *tools.ExecuteResult) {
	if host == nil {
		return subagentInput{}, &tools.ExecuteResult{Output: "subagent execution is not configured"}
	}
	input := subagentInput{
		title:   anyToTrimmedString(r.params["title"]),
		task:    anyToTrimmedString(r.params["task"]),
		context: anyToTrimmedString(r.params["context"]),
	}
	depth, maxDepth := r.opts.Depth, constants.MaxSubagentDepth
	if loop != nil {
		if input.task == "" || len(input.task)+len(input.context) > 16384 {
			return subagentInput{}, &tools.ExecuteResult{Error: "task must be nonempty and at most 16 KiB"}
		}
		depth, maxDepth = loop.caller.Depth, loop.caller.MaxDepth
		if depth == 0 && maxDepth == 0 {
			return subagentInput{}, &tools.ExecuteResult{Error: "delegation is disabled"}
		}
	}
	if depth >= maxDepth {
		if loop != nil {
			return subagentInput{}, &tools.ExecuteResult{Error: "delegation depth limit exceeded"}
		}
		return subagentInput{}, &tools.ExecuteResult{Output: "nested run_subagent is not allowed"}
	}
	if input.task == "" {
		return subagentInput{}, &tools.ExecuteResult{Output: "task is required"}
	}
	return input, nil
}

func (r subagentExecutionRequest) resolveModel(ctx context.Context, host SubagentHost) (llmsvc.ModelRuntimeConfig, error) {
	// The user selection takes priority over the inherited model. Ignore model_id
	// in tool arguments so the model cannot choose a configuration on its own.
	userOverride := strings.TrimSpace(r.opts.SubagentModelID)
	if userOverride == "" {
		return r.parentModel, nil
	}
	resolved, err := host.ResolveModelRuntimeConfig(ctx, userOverride)
	if err != nil {
		return llmsvc.ModelRuntimeConfig{}, fmt.Errorf("failed to resolve user subagent model: %w", err)
	}
	resolved.ThinkingLevel = r.parentModel.ThinkingLevel
	return resolved, nil
}

func (r *subagentExecutionRequest) reserveParallelMember(ctx context.Context, host SubagentHost) {
	if r.opts.teamRuntime == nil || r.opts.Depth >= constants.MaxSubagentDepth {
		return
	}
	input, rejection := r.admit(host, r.opts.Cooperative)
	if rejection != nil {
		return
	}
	model, err := r.resolveModel(ctx, host)
	if err != nil {
		r.reservationErr = err
		return
	}
	r.reservedMember, r.reservationErr = r.reserveMember(ctx, normalizeSubagentTitle(input.title, input.task), input.task, model.ConfigID)
}

func (r subagentExecutionRequest) reserveMember(ctx context.Context, title, task, modelID string) (*domain.TeamMemberRun, error) {
	if r.reservationErr != nil || r.reservedMember != nil || r.opts.teamRuntime == nil {
		return r.reservedMember, r.reservationErr
	}
	return r.opts.teamRuntime.reserveMember(ctx, r.toolCall.ID, title, task, modelID, r.callbacks)
}

func normalizeSubagentTitle(title, task string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if normalized == "" {
		normalized = strings.Join(strings.Fields(strings.TrimSpace(task)), " ")
	}
	runes := []rune(normalized)
	if len(runes) <= maxSubagentTitleRunes {
		return normalized
	}
	return string(runes[:maxSubagentTitleRunes-3]) + "..."
}

func anyToTrimmedString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

type subagentExecution struct {
	id, title, task, modelID string
	member                   *domain.TeamMemberRun
}

type subagentOutcome struct {
	answer   string
	err      error
	canceled bool
}

func (r subagentExecutionRequest) runWithLifecycle(
	ctx context.Context,
	execution subagentExecution,
	run func(context.Context, AgentEventMeta) subagentOutcome,
) (*tools.ExecuteResult, error) {
	meta := AgentEventMeta{ParentToolCallID: r.toolCall.ID, SubagentRunID: execution.id}
	if execution.member != nil {
		started, err := r.opts.teamRuntime.startMember(ctx, execution.member.ID, execution.id, execution.modelID)
		if err != nil {
			return &tools.ExecuteResult{Error: err.Error()}, nil
		}
		meta.TeamRunID = started.TeamRunID
		meta.MemberRunID = started.ID
	}
	cooperative := r.opts.Cooperative != nil
	if r.callbacks.OnSubagentStart != nil {
		if err := r.callbacks.OnSubagentStart(meta, execution.title, execution.task); err != nil {
			_ = r.finishMember(ctx, execution.member, subagentOutcome{err: err})
			if cooperative {
				return nil, err
			}
			return nil, fmt.Errorf("failed to push subagent start: %w", err)
		}
	}

	outcome := run(ctx, meta)
	// The cooperative compatibility event historically precedes the team update.
	if !cooperative {
		if err := r.finishMember(ctx, execution.member, outcome); err != nil && outcome.err == nil {
			outcome.err = err
		}
	}
	if r.callbacks.OnSubagentDone != nil {
		_ = r.callbacks.OnSubagentDone(meta, outcome.err)
	}
	if cooperative {
		_ = r.finishMember(ctx, execution.member, outcome)
	}
	result := &tools.ExecuteResult{Output: outcome.answer}
	if outcome.err != nil {
		result.Error = outcome.err.Error()
	}
	return result, nil
}

func (r subagentExecutionRequest) finishMember(ctx context.Context, member *domain.TeamMemberRun, outcome subagentOutcome) error {
	if member == nil {
		return nil
	}
	result := teamsvc.MemberResult{Answer: outcome.answer, Err: outcome.err}
	if outcome.canceled || errors.Is(outcome.err, context.Canceled) || errors.Is(outcome.err, context.DeadlineExceeded) {
		result.Err = nil
		result.Canceled = true
	}
	_, err := r.opts.teamRuntime.finishMember(context.WithoutCancel(ctx), member.ID, result)
	return err
}

type directSubagentExecutor struct {
	agent *AgentService
}

func (e directSubagentExecutor) ExecuteSubagent(ctx context.Context, r subagentExecutionRequest) (*tools.ExecuteResult, error) {
	host := e.agent.subagentHost
	input, rejection := r.admit(host, nil)
	if rejection != nil {
		return rejection, nil
	}
	if r.callbacks.OnToolCallStart != nil {
		request := (toolInvocation{call: r.toolCall, resolvedToolInvocation: r.invocation, params: r.params}).startRequest()
		request.ReviewStatus = ""
		request.Preamble = r.preamble
		if err := r.callbacks.OnToolCallStart(request); err != nil {
			return nil, fmt.Errorf("failed to push tool approval request: %w", err)
		}
	}
	approved, message, _ := waitApprovalIfNeeded(ctx, r.callbacks, r.toolCall, r.invocation, r.params, r.preamble, nil)
	if !approved {
		return &tools.ExecuteResult{Output: message}, nil
	}

	model, err := r.resolveModel(ctx, host)
	if err != nil {
		return &tools.ExecuteResult{Error: err.Error()}, nil
	}
	title := normalizeSubagentTitle(input.title, input.task)
	member, err := r.reserveMember(ctx, title, input.task, model.ConfigID)
	if err != nil {
		return &tools.ExecuteResult{Error: err.Error()}, nil
	}
	messages, err := host.BuildSubagentMessages(ctx, r.sessionID, input.task, input.context)
	if err != nil {
		return &tools.ExecuteResult{Error: fmt.Sprintf("failed to build subagent context: %s", err)}, nil
	}

	execution := subagentExecution{id: uuid.NewString(), title: title, task: input.task, modelID: model.ConfigID, member: member}
	return r.runWithLifecycle(ctx, execution, func(ctx context.Context, meta AgentEventMeta) subagentOutcome {
		callbacks := wrapSubagentCallbacks(r.callbacks, meta)
		opts := AgentLoopOptions{
			Depth: r.opts.Depth + 1, ApprovalMode: r.opts.ApprovalMode, PlanMode: r.opts.PlanMode,
			SandboxPolicy: r.opts.SandboxPolicy,
		}
		answer, err := e.agent.RunAgentLoop(ctx, model, r.sessionID, messages, r.mcpConfigs, r.activatedSkills, callbacks, opts)
		return subagentOutcome{answer: strings.TrimSpace(answer), err: err}
	})
}

type cooperativeSubagentExecutor struct {
	host SubagentHost
	loop *cooperativeLoop
}

func (e cooperativeSubagentExecutor) ExecuteSubagent(ctx context.Context, r subagentExecutionRequest) (*tools.ExecuteResult, error) {
	c := e.loop
	input, rejection := r.admit(e.host, c)
	if rejection != nil {
		return rejection, nil
	}
	if r.reservationErr != nil {
		return &tools.ExecuteResult{Error: r.reservationErr.Error()}, nil
	}
	d, err := c.service.Spawn(ctx, c.caller, subagent.Spawn{
		Title: input.title, Task: input.task, Context: input.context,
		ModelID: c.model.ConfigID, Workspace: c.workspace, OneShot: true,
	})
	if err != nil {
		return &tools.ExecuteResult{Error: err.Error()}, nil
	}
	defer func() {
		// A failed callback or canceled caller still owns this one-shot child.
		if c.service.Runtime.IsLive(d.SessionID) {
			finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = c.service.Runtime.Interrupt(finish, d.SessionID, true)
		}
	}()

	member, err := r.reserveMember(ctx, d.Title, d.Task, c.model.ConfigID)
	if err != nil {
		return &tools.ExecuteResult{Error: err.Error()}, nil
	}
	execution := subagentExecution{id: d.SessionID, title: d.Title, task: d.Task, modelID: c.model.ConfigID, member: member}
	return r.runWithLifecycle(ctx, execution, func(ctx context.Context, meta AgentEventMeta) subagentOutcome {
		return e.waitForResult(ctx, d.SessionID, wrapSubagentCallbacks(r.callbacks, meta))
	})
}

func (e cooperativeSubagentExecutor) waitForResult(ctx context.Context, id string, callbacks AgentCallbacks) subagentOutcome {
	runtime := e.loop.service.Runtime
	for {
		changed := runtime.Changes()
		if !runtime.IsLive(id) {
			break
		}
		if err := runtime.WaitSignal(ctx, changed, time.Second); err != nil {
			return subagentOutcome{err: err}
		}
	}
	turns, err := e.loop.service.Store.ListAgentTurns(context.WithoutCancel(ctx), id)
	if err != nil {
		return subagentOutcome{err: err}
	}
	if len(turns) == 0 {
		return subagentOutcome{err: errors.New("subagent did not produce a terminal record")}
	}
	turn := turns[0]
	outcome := subagentOutcome{answer: turn.Answer, canceled: turn.Status == "canceled"}
	if turn.Status != "succeeded" {
		outcome.err = fmt.Errorf("%s: %s", turn.StopReason, turn.Error)
	}
	if turn.Thinking != "" {
		_ = callbacks.OnThinkingStart(ThinkingEventMeta{})
		_ = callbacks.OnThinkingChunk(turn.Thinking, ThinkingEventMeta{})
		_ = callbacks.OnThinkingDone(ThinkingEventMeta{})
	}
	if turn.Answer != "" {
		_ = callbacks.OnChunk(turn.Answer)
	}
	return outcome
}
