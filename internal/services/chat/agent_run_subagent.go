package chat

import (
	"context"
	"errors"
	"fmt"
	subagent "slimebot/internal/services/subagent"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	teamsvc "slimebot/internal/services/team"
	"slimebot/internal/tools"

	"github.com/google/uuid"
)

const maxSubagentTitleRunes = 80

type agentSubagentRunner struct {
	agent               *AgentService
	parentModel         llmsvc.ModelRuntimeConfig
	sessionID           string
	mcpConfigs          []domain.MCPConfig
	activatedSkills     map[string]struct{}
	callbacks           AgentCallbacks
	opts                AgentLoopOptions
	toolCall            llmsvc.ToolCallInfo
	invocation          resolvedToolInvocation
	userSubagentModelID string
	preamble            string
	reservedMember      *domain.TeamMemberRun
	reservationErr      error
}

func (r agentSubagentRunner) RunSubagent(ctx context.Context, request tools.SubagentRunRequest) (*tools.ExecuteResult, error) {
	params := request.Params
	if params == nil {
		params = map[string]any{
			"title":   request.Title,
			"task":    request.Task,
			"context": request.Context,
		}
	}
	return r.agent.executeRunSubagentTool(
		ctx,
		r.parentModel,
		r.sessionID,
		r.mcpConfigs,
		r.activatedSkills,
		r.callbacks,
		r.opts,
		r.toolCall,
		r.invocation,
		params,
		r.userSubagentModelID,
		r.preamble,
		r.reservedMember,
		r.reservationErr,
	)
}

func (a *AgentService) reserveParallelSubagentMember(
	ctx context.Context,
	parentModel llmsvc.ModelRuntimeConfig,
	opts AgentLoopOptions,
	tc llmsvc.ToolCallInfo,
	params map[string]any,
	callbacks AgentCallbacks,
) (*domain.TeamMemberRun, error) {
	if opts.teamRuntime == nil || a.subagentHost == nil || opts.Depth >= constants.MaxSubagentDepth {
		return nil, nil
	}
	task := anyToTrimmedString(params["task"])
	if task == "" {
		return nil, nil
	}
	modelConfigID := parentModel.ConfigID
	if userOverride := strings.TrimSpace(opts.SubagentModelID); userOverride != "" {
		resolved, err := a.subagentHost.ResolveModelRuntimeConfig(ctx, userOverride)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve user subagent model: %w", err)
		}
		modelConfigID = resolved.ConfigID
	}
	title := normalizeSubagentTitle(anyToTrimmedString(params["title"]), task)
	return opts.teamRuntime.reserveMember(ctx, tc.ID, title, task, modelConfigID, callbacks)
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
	if maxSubagentTitleRunes <= 3 {
		return string(runes[:maxSubagentTitleRunes])
	}
	return string(runes[:maxSubagentTitleRunes-3]) + "..."
}

func anyToTrimmedString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func cloneActivatedSkills(activated map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(activated))
	for name := range activated {
		cloned[name] = struct{}{}
	}
	return cloned
}

func mergeActivatedSkills(dst, src map[string]struct{}) {
	for name := range src {
		dst[name] = struct{}{}
	}
}

func (a *AgentService) handleRunSubagentTool(
	ctx context.Context,
	parentModel llmsvc.ModelRuntimeConfig,
	sessionID string,
	mcpConfigs []domain.MCPConfig,
	activatedSkills map[string]struct{},
	callbacks AgentCallbacks,
	opts AgentLoopOptions,
	tc llmsvc.ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]any,
	userSubagentModelID string,
	preamble string,
	messages *[]llmsvc.ChatMessage,
) error {
	execResult, err := a.executeRunSubagentTool(ctx, parentModel, sessionID, mcpConfigs, activatedSkills, callbacks, opts, tc, invocation, params, userSubagentModelID, preamble, nil, nil)
	if err != nil {
		return err
	}
	resultStatus := buildToolResultStatus(execResult)
	notifyToolResult(callbacks, ToolCallResult{
		ToolCallID:       tc.ID,
		ToolName:         invocation.toolName,
		Command:          invocation.command,
		ModelFuncName:    invocation.modelFuncName,
		RequiresApproval: invocation.requiresApproval,
		Status:           resultStatus,
		Output:           execResult.Output,
		Error:            execResult.Error,
	})
	*messages = appendToolMessage(*messages, tc.ID, buildToolResultContent(execResult))
	return nil
}

func (a *AgentService) executeRunSubagentTool(
	ctx context.Context,
	parentModel llmsvc.ModelRuntimeConfig,
	sessionID string,
	mcpConfigs []domain.MCPConfig,
	activatedSkills map[string]struct{},
	callbacks AgentCallbacks,
	opts AgentLoopOptions,
	tc llmsvc.ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]any,
	userSubagentModelID string,
	preamble string,
	reservedMember *domain.TeamMemberRun,
	reservationErr error,
) (*tools.ExecuteResult, error) {
	if a.subagentHost == nil {
		return &tools.ExecuteResult{Output: "subagent execution is not configured"}, nil
	}
	if opts.Cooperative != nil {
		if reservationErr != nil {
			return &tools.ExecuteResult{Error: reservationErr.Error()}, nil
		}
		c := opts.Cooperative
		d, err := c.service.Spawn(ctx, c.caller, subagent.Spawn{Title: anyToTrimmedString(params["title"]), Task: anyToTrimmedString(params["task"]), Context: anyToTrimmedString(params["context"]), ModelID: c.model.ConfigID, Workspace: c.workspace, OneShot: true})
		if err != nil {
			return &tools.ExecuteResult{Error: err.Error()}, nil
		}
		// Compatibility callbacks can fail after admission; do not leave an
		// invisible one-shot activation behind on those error paths.
		defer func() {
			if c.service.Runtime.IsLive(d.SessionID) {
				finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				_ = c.service.Runtime.Interrupt(finish, d.SessionID, true)
			}
		}()
		member := reservedMember
		if opts.teamRuntime != nil && member == nil {
			member, err = opts.teamRuntime.reserveMember(ctx, tc.ID, d.Title, d.Task, c.model.ConfigID, callbacks)
			if err != nil {
				return &tools.ExecuteResult{Error: err.Error()}, nil
			}
		}
		meta := AgentEventMeta{ParentToolCallID: tc.ID, SubagentRunID: d.SessionID}
		if member != nil {
			started, e := opts.teamRuntime.startMember(ctx, member.ID, d.SessionID, c.model.ConfigID)
			if e != nil {
				return &tools.ExecuteResult{Error: e.Error()}, nil
			}
			meta.TeamRunID = started.TeamRunID
			meta.MemberRunID = started.ID
		}
		if callbacks.OnSubagentStart != nil {
			if e := callbacks.OnSubagentStart(meta, d.Title, d.Task); e != nil {
				return nil, e
			}
		}
		for c.service.Runtime.IsLive(d.SessionID) {
			if e := c.service.Runtime.Wait(ctx, c.caller.RootID, nil, time.Second); e != nil {
				return nil, e
			}
		}
		turns, e := c.service.Store.ListAgentTurns(context.WithoutCancel(ctx), d.SessionID)
		if e != nil {
			return nil, e
		}
		if len(turns) == 0 {
			return &tools.ExecuteResult{Error: "subagent did not produce a terminal record"}, nil
		}
		t := turns[0]
		var runErr error
		if t.Status != "succeeded" {
			runErr = fmt.Errorf("%s: %s", t.StopReason, t.Error)
		}
		if t.Thinking != "" {
			childCB := wrapSubagentCallbacks(callbacks, meta)
			if childCB.OnThinkingStart != nil {
				_ = childCB.OnThinkingStart(ThinkingEventMeta{})
			}
			if childCB.OnThinkingChunk != nil {
				_ = childCB.OnThinkingChunk(t.Thinking, ThinkingEventMeta{})
			}
			if childCB.OnThinkingDone != nil {
				_ = childCB.OnThinkingDone(ThinkingEventMeta{})
			}
		}
		if callbacks.OnSubagentChunk != nil && t.Answer != "" {
			_ = callbacks.OnSubagentChunk(meta, t.Answer)
		}
		if callbacks.OnSubagentDone != nil {
			_ = callbacks.OnSubagentDone(meta, runErr)
		}
		if member != nil {
			_, _ = opts.teamRuntime.finishMember(context.WithoutCancel(ctx), member.ID, teamsvc.MemberResult{Answer: t.Answer, Err: runErr})
		}
		result := &tools.ExecuteResult{Output: t.Answer}
		if runErr != nil {
			result.Error = runErr.Error()
		}
		return result, nil
	}
	if opts.Depth >= constants.MaxSubagentDepth {
		return &tools.ExecuteResult{Output: "nested run_subagent is not allowed"}, nil
	}

	task := anyToTrimmedString(params["task"])
	if task == "" {
		return &tools.ExecuteResult{Output: "task is required"}, nil
	}

	if callbacks.OnToolCallStart != nil {
		if err := callbacks.OnToolCallStart(ApprovalRequest{
			ToolCallID:       tc.ID,
			ToolName:         invocation.toolName,
			Command:          invocation.command,
			ModelFuncName:    invocation.modelFuncName,
			Params:           params,
			RequiresApproval: invocation.requiresApproval,
			Preamble:         preamble,
		}); err != nil {
			return nil, fmt.Errorf("failed to push tool approval request: %w", err)
		}
	}

	approved, rejectionMessage, _ := waitApprovalIfNeeded(ctx, callbacks, tc, invocation, params, preamble, nil)
	if !approved {
		return &tools.ExecuteResult{Output: rejectionMessage}, nil
	}

	parentCtx := anyToTrimmedString(params["context"])
	subModel := parentModel
	title := normalizeSubagentTitle(anyToTrimmedString(params["title"]), task)

	// Priority: user UI/config selection > inherit parent model. Ignore any LLM-supplied
	// model_id argument so the model cannot invent aliases such as "fast".
	if userOverride := strings.TrimSpace(userSubagentModelID); userOverride != "" {
		resolved, err := a.subagentHost.ResolveModelRuntimeConfig(ctx, userOverride)
		if err != nil {
			msg := fmt.Sprintf("failed to resolve user subagent model: %s", err.Error())
			return &tools.ExecuteResult{Error: msg}, nil
		}
		resolved.ThinkingLevel = parentModel.ThinkingLevel
		subModel = resolved
	}

	if reservationErr != nil {
		return &tools.ExecuteResult{Error: reservationErr.Error()}, nil
	}
	member := reservedMember
	if opts.teamRuntime != nil && member == nil {
		reserved, err := opts.teamRuntime.reserveMember(ctx, tc.ID, title, task, subModel.ConfigID, callbacks)
		if err != nil {
			return &tools.ExecuteResult{Error: err.Error()}, nil
		}
		member = reserved
	}

	subMsgs, err := a.subagentHost.BuildSubagentMessages(ctx, sessionID, task, parentCtx)
	if err != nil {
		msg := fmt.Sprintf("failed to build subagent context: %s", err.Error())
		return &tools.ExecuteResult{Error: msg}, nil
	}

	runID := uuid.NewString()
	meta := AgentEventMeta{ParentToolCallID: tc.ID, SubagentRunID: runID}
	if member != nil {
		started, startErr := opts.teamRuntime.startMember(ctx, member.ID, runID, subModel.ConfigID)
		if startErr != nil {
			return &tools.ExecuteResult{Error: startErr.Error()}, nil
		}
		meta.TeamRunID = started.TeamRunID
		meta.MemberRunID = started.ID
	}
	if callbacks.OnSubagentStart != nil {
		if err := callbacks.OnSubagentStart(meta, title, task); err != nil {
			if member != nil {
				_, _ = opts.teamRuntime.finishMember(context.Background(), member.ID, teamsvc.MemberResult{Err: err})
			}
			return nil, fmt.Errorf("failed to push subagent start: %w", err)
		}
	}

	subCb := wrapSubagentCallbacks(callbacks, meta)
	childOpts := AgentLoopOptions{Depth: opts.Depth + 1, ApprovalMode: opts.ApprovalMode, PlanMode: opts.PlanMode, SandboxPolicy: opts.SandboxPolicy}

	answer, runErr := a.RunAgentLoop(ctx, subModel, sessionID, subMsgs, mcpConfigs, activatedSkills, subCb, childOpts)
	if member != nil {
		finishCtx := ctx
		if ctx.Err() != nil {
			finishCtx = context.Background()
		}
		memberResult := teamsvc.MemberResult{Answer: answer, Err: runErr}
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			memberResult.Err = nil
			memberResult.Canceled = true
		}
		if _, finishErr := opts.teamRuntime.finishMember(finishCtx, member.ID, memberResult); finishErr != nil && runErr == nil {
			runErr = finishErr
		}
	}

	if callbacks.OnSubagentDone != nil {
		_ = callbacks.OnSubagentDone(meta, runErr)
	}

	var execResult *tools.ExecuteResult
	if runErr != nil {
		execResult = &tools.ExecuteResult{Output: strings.TrimSpace(answer), Error: runErr.Error()}
	} else {
		execResult = &tools.ExecuteResult{Output: strings.TrimSpace(answer), Error: ""}
	}

	return execResult, nil
}
