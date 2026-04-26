package chat

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
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
	requiresApproval bool
}

// resolveToolInvocation normalizes a model function name into a tool invocation.
func resolveToolInvocation(tc llmsvc.ToolCallInfo, mcpToolMeta map[string]mcp.ToolMeta, approvalMode string) (resolvedToolInvocation, error) {
	if tc.Name == constants.ActivateSkillTool {
		return resolvedToolInvocation{
			toolName:         constants.ActivateSkillTool,
			command:          "activate",
			isMCP:            false,
			requiresApproval: false,
		}, nil
	}
	if tc.Name == constants.SearchMemoryTool {
		// Memory search uses the built-in tool path; no approval.
		return resolvedToolInvocation{
			toolName:         constants.SearchMemoryTool,
			command:          "query",
			isMCP:            false,
			requiresApproval: false,
		}, nil
	}
	if tc.Name == constants.RunSubagentTool {
		return resolvedToolInvocation{
			toolName:         constants.RunSubagentTool,
			command:          "run",
			isMCP:            false,
			requiresApproval: false,
		}, nil
	}
	toolName, command, err := parseToolCallName(tc.Name)
	if mcpMeta, ok := mcpToolMeta[tc.Name]; ok {
		return resolvedToolInvocation{
			toolName:         mcpMeta.ServerAlias,
			command:          mcpMeta.ToolName,
			isMCP:            true,
			requiresApproval: requiresToolApproval(mcpMeta.ServerAlias, true, approvalMode),
		}, nil
	}
	if err != nil {
		return resolvedToolInvocation{}, err
	}
	return resolvedToolInvocation{
		toolName:         toolName,
		command:          command,
		isMCP:            false,
		requiresApproval: requiresToolApproval(toolName, false, approvalMode),
	}, nil
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
	params map[string]string,
	preamble string,
) (bool, string, string) {
	if !invocation.requiresApproval {
		return true, "", ""
	}
	approvalCtx, cancel := context.WithTimeout(ctx, constants.AgentApprovalTimeout)
	defer cancel()

	approval, err := callbacks.WaitApproval(approvalCtx, tc.ID)
	if err != nil {
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID: tc.ID, ToolName: invocation.toolName, Command: invocation.command,
			RequiresApproval: invocation.requiresApproval, Status: constants.ToolCallStatusError, Error: "Approval timed out.",
		})
		return false, "Approval timed out or failed. The tool call was cancelled.", ""
	}
	if !approval.Approved {
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID: tc.ID, ToolName: invocation.toolName, Command: invocation.command,
			RequiresApproval: invocation.requiresApproval, Status: constants.ToolCallStatusRejected, Error: "Execution was rejected by the user.",
		})
		return false, "The user rejected this tool call. Please answer in another way or explain that authorization is required.", ""
	}
	_ = params
	_ = preamble
	return true, "", approval.Answers
}

// executeInvocation dispatches to MCP, memory, or built-in tool execution.
func (a *AgentService) executeInvocation(
	ctx context.Context,
	tc llmsvc.ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]string,
	sessionID string,
	mcpConfigs []domain.MCPConfig,
	memoryToolUsed *bool,
) *tools.ExecuteResult {
	if invocation.isMCP {
		argsAny, parseErr := parseToolCallArgsAny(tc.Arguments)
		if parseErr != nil {
			return &tools.ExecuteResult{Error: parseErr.Error()}
		}
		callResult, callErr := a.mcp.Execute(ctx, mcpConfigs, invocation.toolName, invocation.command, argsAny)
		if callErr != nil {
			return &tools.ExecuteResult{Error: callErr.Error()}
		}
		return &tools.ExecuteResult{Output: callResult.Output, Error: callResult.Error}
	}

	if (invocation.toolName == "memory" && invocation.command == "query") ||
		(invocation.toolName == constants.SearchMemoryTool && invocation.command == "query") {
		// Allow at most one memory tool call per assistant turn to avoid duplicate context noise.
		if *memoryToolUsed {
			return &tools.ExecuteResult{Error: "search_memory can be called at most once per response."}
		}
		if a.memory == nil {
			return &tools.ExecuteResult{Error: "Memory service is not enabled."}
		}
		*memoryToolUsed = true
		queryResult, queryErr := a.memory.QueryForAgent(ctx, sessionID, params["query"], constants.MemoryToolDefaultTopK)
		if queryErr != nil {
			return &tools.ExecuteResult{Output: queryResult.Output, Error: queryErr.Error()}
		}
		return &tools.ExecuteResult{Output: queryResult.Output}
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
