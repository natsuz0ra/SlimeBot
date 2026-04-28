package chat

import (
	"context"
	"fmt"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	"slimebot/internal/tools"

	"github.com/google/uuid"
)

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
	params map[string]string,
	preamble string,
	messages *[]llmsvc.ChatMessage,
) error {
	if a.subagentHost == nil {
		*messages = appendToolMessage(*messages, tc.ID, "subagent execution is not configured")
		return nil
	}
	if opts.Depth >= constants.MaxSubagentDepth {
		*messages = appendToolMessage(*messages, tc.ID, "nested run_subagent is not allowed")
		return nil
	}

	task := strings.TrimSpace(params["task"])
	if task == "" {
		*messages = appendToolMessage(*messages, tc.ID, "task is required")
		return nil
	}

	if callbacks.OnToolCallStart != nil {
		if err := callbacks.OnToolCallStart(ApprovalRequest{
			ToolCallID:       tc.ID,
			ToolName:         invocation.toolName,
			Command:          invocation.command,
			Params:           params,
			RequiresApproval: invocation.requiresApproval,
			Preamble:         preamble,
		}); err != nil {
			return fmt.Errorf("failed to push tool approval request: %w", err)
		}
	}

	approved, rejectionMessage, _ := waitApprovalIfNeeded(ctx, callbacks, tc, invocation, params, preamble)
	if !approved {
		*messages = appendToolMessage(*messages, tc.ID, rejectionMessage)
		return nil
	}

	parentCtx := strings.TrimSpace(params["context"])
	modelOverride := strings.TrimSpace(params["model_id"])
	subModel := parentModel
	if modelOverride != "" {
		resolved, err := a.subagentHost.ResolveModelRuntimeConfig(ctx, modelOverride)
		if err != nil {
			msg := fmt.Sprintf("failed to resolve model_id: %s", err.Error())
			notifyToolResult(callbacks, ToolCallResult{
				ToolCallID:       tc.ID,
				ToolName:         invocation.toolName,
				Command:          invocation.command,
				RequiresApproval: invocation.requiresApproval,
				Status:           constants.ToolCallStatusError,
				Output:           "",
				Error:            msg,
			})
			*messages = appendToolMessage(*messages, tc.ID, msg)
			return nil
		}
		subModel = resolved
	}

	subMsgs, err := a.subagentHost.BuildSubagentMessages(ctx, sessionID, task, parentCtx)
	if err != nil {
		msg := fmt.Sprintf("failed to build subagent context: %s", err.Error())
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID:       tc.ID,
			ToolName:         invocation.toolName,
			Command:          invocation.command,
			RequiresApproval: invocation.requiresApproval,
			Status:           constants.ToolCallStatusError,
			Output:           "",
			Error:            msg,
		})
		*messages = appendToolMessage(*messages, tc.ID, msg)
		return nil
	}

	runID := uuid.NewString()
	if callbacks.OnSubagentStart != nil {
		_ = callbacks.OnSubagentStart(tc.ID, runID, task)
	}

	subCb := wrapSubagentCallbacks(callbacks, tc.ID, runID)
	childOpts := AgentLoopOptions{Depth: opts.Depth + 1, ApprovalMode: opts.ApprovalMode, PlanMode: opts.PlanMode}

	answer, runErr := a.RunAgentLoop(ctx, subModel, sessionID, subMsgs, mcpConfigs, activatedSkills, subCb, childOpts)

	if callbacks.OnSubagentDone != nil {
		_ = callbacks.OnSubagentDone(tc.ID, runID, runErr)
	}

	var execResult *tools.ExecuteResult
	if runErr != nil {
		execResult = &tools.ExecuteResult{Output: strings.TrimSpace(answer), Error: runErr.Error()}
	} else {
		execResult = &tools.ExecuteResult{Output: strings.TrimSpace(answer), Error: ""}
	}

	resultStatus := buildToolResultStatus(execResult)
	notifyToolResult(callbacks, ToolCallResult{
		ToolCallID:       tc.ID,
		ToolName:         invocation.toolName,
		Command:          invocation.command,
		RequiresApproval: invocation.requiresApproval,
		Status:           resultStatus,
		Output:           execResult.Output,
		Error:            execResult.Error,
	})

	*messages = appendToolMessage(*messages, tc.ID, buildToolResultContent(execResult))
	return nil
}
