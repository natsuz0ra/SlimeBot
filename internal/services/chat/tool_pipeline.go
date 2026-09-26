package chat

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"slimebot/internal/sandbox"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/mcp"
	llmsvc "slimebot/internal/services/llm"
	"slimebot/internal/tools"
)

type resolvedToolInvocation struct {
	toolName         string
	command          string
	isMCP            bool
	serverAlias      string
	modelFuncName    string
	requiresApproval bool
	approvalPolicy   toolApprovalPolicy
}

type toolApprovalPolicy string

const (
	toolApprovalPolicyNone       toolApprovalPolicy = "none"
	toolApprovalPolicyManual     toolApprovalPolicy = "manual"
	toolApprovalPolicyAutoReview toolApprovalPolicy = "auto_review"
)

// resolveToolInvocation normalizes a model function name into a tool invocation.
func resolveToolInvocation(tc llmsvc.ToolCallInfo, mcpToolMeta map[string]mcp.ToolMeta, approvalMode string) (resolvedToolInvocation, error) {
	if meta, ok := tools.MetadataForFunction(tc.Name); ok && meta.StableName && meta.DefaultCommand != "" {
		policy := determineToolApprovalPolicy(meta.Name, false, approvalMode)
		return resolvedToolInvocation{
			toolName:         meta.Name,
			command:          meta.DefaultCommand,
			isMCP:            false,
			modelFuncName:    tc.Name,
			requiresApproval: policy != toolApprovalPolicyNone,
			approvalPolicy:   policy,
		}, nil
	}
	toolName, command, err := parseToolCallName(tc.Name)
	if mcpMeta, ok := mcpToolMeta[tc.Name]; ok {
		policy := determineToolApprovalPolicy(mcpMeta.ServerAlias, true, approvalMode)
		serverName := strings.TrimSpace(mcpMeta.ServerName)
		if serverName == "" {
			serverName = mcpMeta.ServerAlias
		}
		modelFuncName := strings.TrimSpace(mcpMeta.FuncName)
		if modelFuncName == "" {
			modelFuncName = tc.Name
		}
		return resolvedToolInvocation{
			toolName:         serverName,
			command:          mcpMeta.ToolName,
			isMCP:            true,
			serverAlias:      mcpMeta.ServerAlias,
			modelFuncName:    modelFuncName,
			requiresApproval: policy != toolApprovalPolicyNone,
			approvalPolicy:   policy,
		}, nil
	}
	if err != nil {
		return resolvedToolInvocation{}, err
	}
	policy := determineToolApprovalPolicy(toolName, false, approvalMode)
	return resolvedToolInvocation{
		toolName:         toolName,
		command:          command,
		isMCP:            false,
		modelFuncName:    tc.Name,
		requiresApproval: policy != toolApprovalPolicyNone,
		approvalPolicy:   policy,
	}, nil
}

func applyParamApprovalPolicy(invocation resolvedToolInvocation, params map[string]any) resolvedToolInvocation {
	if invocation.toolName == constants.ExecToolName && strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", params["sandbox_permissions"])), "required_approval") {
		invocation.requiresApproval = true
		invocation.approvalPolicy = toolApprovalPolicyManual
	}
	return invocation
}

// notifyToolResult wraps the tool-result callback with consistent logging on failure.
func notifyToolResult(callbacks AgentCallbacks, result ToolCallResult) {
	if callbacks.OnToolCallResult == nil {
		return
	}
	if err := callbacks.OnToolCallResult(result); err != nil {
		logging.Warn("failed_to_push_tool_result", "err", err)
	}
}

// waitApprovalIfNeeded blocks for frontend approval when required; returns (approved, rejectionMessage, answers).
func waitApprovalIfNeeded(
	ctx context.Context,
	callbacks AgentCallbacks,
	tc llmsvc.ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]any,
	preamble string,
	review func(context.Context, ApprovalReviewRequest) (*ApprovalReviewResult, error),
) (bool, string, string) {
	if !invocation.requiresApproval {
		return true, "", ""
	}
	if invocation.approvalPolicy == toolApprovalPolicyAutoReview {
		reviewResult := runApprovalReview(ctx, callbacks, tc, invocation, params, preamble, review)
		if reviewResult != nil && reviewResult.Decision == ApprovalReviewDecisionApprove {
			return true, "", ""
		}
		if callbacks.OnToolApprovalRequired != nil {
			if err := callbacks.OnToolApprovalRequired(ApprovalRequest{
				ToolCallID:       tc.ID,
				ToolName:         invocation.toolName,
				Command:          invocation.command,
				ModelFuncName:    invocation.modelFuncName,
				Params:           params,
				RequiresApproval: true,
				ReviewStatus:     string(ApprovalReviewStatusNeedsUser),
				ReviewRisk:       reviewResultRisk(reviewResult),
				ReviewReason:     reviewResultReason(reviewResult),
				Preamble:         preamble,
			}); err != nil {
				notifyToolResult(callbacks, ToolCallResult{
					ToolCallID:       tc.ID,
					ToolName:         invocation.toolName,
					Command:          invocation.command,
					ModelFuncName:    invocation.modelFuncName,
					RequiresApproval: invocation.requiresApproval,
					Status:           constants.ToolCallStatusError,
					Error:            "Approval review failed to request user approval.",
				})
				return false, "Approval review failed to request user approval. The tool call was cancelled.", ""
			}
		}
	}
	approvalCtx, cancel := context.WithTimeout(ctx, constants.AgentApprovalTimeout)
	defer cancel()

	approval, err := callbacks.WaitApproval(approvalCtx, tc.ID)
	if err != nil {
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID:       tc.ID,
			ToolName:         invocation.toolName,
			Command:          invocation.command,
			ModelFuncName:    invocation.modelFuncName,
			RequiresApproval: invocation.requiresApproval,
			Status:           constants.ToolCallStatusError,
			Error:            "Approval timed out.",
		})
		return false, "Approval timed out or failed. The tool call was cancelled.", ""
	}
	if !approval.Approved {
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID:       tc.ID,
			ToolName:         invocation.toolName,
			Command:          invocation.command,
			ModelFuncName:    invocation.modelFuncName,
			RequiresApproval: invocation.requiresApproval,
			Status:           constants.ToolCallStatusRejected,
			Error:            "Execution was rejected by the user.",
		})
		return false, "The user rejected this tool call. Please answer in another way or explain that authorization is required.", ""
	}
	_ = params
	_ = preamble
	return true, "", approval.Answers
}

func runApprovalReview(
	ctx context.Context,
	callbacks AgentCallbacks,
	tc llmsvc.ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]any,
	preamble string,
	review func(context.Context, ApprovalReviewRequest) (*ApprovalReviewResult, error),
) *ApprovalReviewResult {
	if callbacks.OnToolApprovalReview != nil {
		_ = callbacks.OnToolApprovalReview(ApprovalReviewEvent{
			ToolCallID:   tc.ID,
			ToolName:     invocation.toolName,
			Command:      invocation.command,
			ReviewStatus: string(ApprovalReviewStatusReviewing),
		})
	}
	result := &ApprovalReviewResult{
		Decision: ApprovalReviewDecisionAskUser,
		Risk:     "unknown",
		Reason:   "Automatic approval review is unavailable.",
	}
	if review != nil {
		reviewCtx, cancel := context.WithTimeout(ctx, constants.AgentApprovalReviewTimeout)
		defer cancel()
		if got, err := review(reviewCtx, ApprovalReviewRequest{
			ToolCallID:       tc.ID,
			ToolName:         invocation.toolName,
			Command:          invocation.command,
			Params:           params,
			Preamble:         preamble,
			WorkingDirectory: sandbox.WorkingDirectoryFromContext(ctx),
		}); err == nil && got != nil {
			result = normalizeApprovalReviewResult(got)
		}
	}
	status := ApprovalReviewStatusNeedsUser
	if result.Decision == ApprovalReviewDecisionApprove {
		status = ApprovalReviewStatusApproved
	}
	if callbacks.OnToolApprovalReview != nil {
		_ = callbacks.OnToolApprovalReview(ApprovalReviewEvent{
			ToolCallID:   tc.ID,
			ToolName:     invocation.toolName,
			Command:      invocation.command,
			ReviewStatus: string(status),
			ReviewRisk:   result.Risk,
			ReviewReason: result.Reason,
		})
	}
	return result
}

func normalizeApprovalReviewResult(result *ApprovalReviewResult) *ApprovalReviewResult {
	if result == nil {
		return &ApprovalReviewResult{Decision: ApprovalReviewDecisionAskUser, Risk: "unknown", Reason: "Automatic approval review returned no decision."}
	}
	decision := result.Decision
	if decision != ApprovalReviewDecisionApprove && decision != ApprovalReviewDecisionAskUser {
		decision = ApprovalReviewDecisionAskUser
	}
	risk := strings.ToLower(strings.TrimSpace(result.Risk))
	switch risk {
	case "low", "medium", "high", "critical":
	default:
		risk = "unknown"
	}
	if decision == ApprovalReviewDecisionApprove && risk != "low" && risk != "medium" {
		decision = ApprovalReviewDecisionAskUser
	}
	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = "Automatic approval review did not provide a reason."
	}
	return &ApprovalReviewResult{Decision: decision, Risk: risk, Reason: reason}
}

func reviewResultRisk(result *ApprovalReviewResult) string {
	if result == nil {
		return "unknown"
	}
	return result.Risk
}

func reviewResultReason(result *ApprovalReviewResult) string {
	if result == nil {
		return "Automatic approval review is unavailable."
	}
	return result.Reason
}

// executeInvocation dispatches to MCP or built-in tool execution.
func (a *AgentService) executeInvocation(
	ctx context.Context,
	tc llmsvc.ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]any,
	sessionID string,
	mcpConfigs []domain.MCPConfig,
) *tools.ExecuteResult {
	_ = sessionID
	if invocation.isMCP {
		argsAny, parseErr := parseToolCallArgsAny(tc.Arguments)
		if parseErr != nil {
			return &tools.ExecuteResult{Error: parseErr.Error()}
		}
		callResult, callErr := a.mcp.Execute(ctx, mcpConfigs, invocation.serverAlias, invocation.command, argsAny)
		if callErr != nil {
			return &tools.ExecuteResult{Error: callErr.Error()}
		}
		return &tools.ExecuteResult{Output: callResult.Output, Error: callResult.Error}
	}

	return executeToolCall(ctx, invocation.toolName, invocation.command, params)
}

// buildToolResultStatus maps execution outcome to the standard status string.
func buildToolResultStatus(execResult *tools.ExecuteResult) string {
	if execResult != nil && strings.TrimSpace(execResult.Error) != "" {
		return constants.ToolCallStatusError
	}
	return constants.ToolCallStatusCompleted
}

// buildToolResultContent builds the tool message body written back into the model context.
func buildToolResultContent(execResult *tools.ExecuteResult) string {
	if execResult == nil {
		return "Execution result:\n"
	}
	if execResult.Error != "" {
		return fmt.Sprintf("Execution result:\n%s\nError: %s", execResult.Output, execResult.Error)
	}
	return fmt.Sprintf("Execution result:\n%s", execResult.Output)
}
