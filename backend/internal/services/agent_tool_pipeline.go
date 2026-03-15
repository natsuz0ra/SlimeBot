package services

import (
	"context"
	"fmt"
	"log"
	"strings"

	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/tools"
)

type resolvedToolInvocation struct {
	toolName         string
	command          string
	isMCP            bool
	requiresApproval bool
}

// resolveToolInvocation 将模型返回的函数名解析成统一工具调用描述。
func resolveToolInvocation(tc ToolCallInfo, mcpToolMeta map[string]mcp.ToolMeta) (resolvedToolInvocation, error) {
	toolName, command, err := parseToolCallName(tc.Name)
	if mcpMeta, ok := mcpToolMeta[tc.Name]; ok {
		return resolvedToolInvocation{
			toolName:         mcpMeta.ServerAlias,
			command:          mcpMeta.ToolName,
			isMCP:            true,
			requiresApproval: requiresToolApproval(mcpMeta.ServerAlias, true),
		}, nil
	}
	if err != nil {
		return resolvedToolInvocation{}, err
	}
	return resolvedToolInvocation{
		toolName:         toolName,
		command:          command,
		isMCP:            false,
		requiresApproval: requiresToolApproval(toolName, false),
	}, nil
}

// notifyToolResult 统一封装工具结果回调，避免主流程重复容错日志。
func notifyToolResult(callbacks AgentCallbacks, result ToolCallResult) {
	if callbacks.OnToolCallResult == nil {
		return
	}
	if err := callbacks.OnToolCallResult(result); err != nil {
		log.Printf("推送工具结果失败: %v", err)
	}
}

// waitApprovalIfNeeded 在需要审批时阻塞等待前端结果，并返回可回填给模型的拒绝原因。
func waitApprovalIfNeeded(
	ctx context.Context,
	callbacks AgentCallbacks,
	tc ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]string,
	preamble string,
) (bool, string) {
	if !invocation.requiresApproval {
		return true, ""
	}
	approvalCtx, cancel := context.WithTimeout(ctx, agentApprovalTimeout)
	defer cancel()

	approval, err := callbacks.WaitApproval(approvalCtx, tc.ID)
	if err != nil {
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID: tc.ID, ToolName: invocation.toolName, Command: invocation.command,
			RequiresApproval: invocation.requiresApproval, Status: "error", Error: "审批超时",
		})
		return false, "用户审批超时或发生错误，工具调用已取消。"
	}
	if !approval.Approved {
		notifyToolResult(callbacks, ToolCallResult{
			ToolCallID: tc.ID, ToolName: invocation.toolName, Command: invocation.command,
			RequiresApproval: invocation.requiresApproval, Status: "rejected", Error: "用户拒绝执行",
		})
		return false, "用户拒绝了此工具调用，请换一种方式回答或告知用户需要授权才能完成此操作。"
	}
	_ = params
	_ = preamble
	return true, ""
}

// executeInvocation 根据调用类型分发到 MCP、memory 或内建工具执行路径。
func (a *AgentService) executeInvocation(
	ctx context.Context,
	tc ToolCallInfo,
	invocation resolvedToolInvocation,
	params map[string]string,
	sessionID string,
	mcpConfigs []models.MCPConfig,
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

	if invocation.toolName == "memory" && invocation.command == "query" {
		if *memoryToolUsed {
			return &tools.ExecuteResult{Error: "memory__query 在单轮回答中最多调用 1 次"}
		}
		if a.memory == nil {
			return &tools.ExecuteResult{Error: "memory service 未启用"}
		}
		*memoryToolUsed = true
		topK := parseOptionalInt(params["top_k"], memoryToolDefaultTopK)
		queryResult, queryErr := a.memory.QueryForAgent(sessionID, params["query"], topK)
		if queryErr != nil {
			return &tools.ExecuteResult{Output: queryResult.Output, Error: queryErr.Error()}
		}
		return &tools.ExecuteResult{Output: queryResult.Output}
	}
	return executeToolCall(invocation.toolName, invocation.command, params)
}

// buildToolResultStatus 将执行结果映射为标准状态字段。
func buildToolResultStatus(execResult *tools.ExecuteResult) string {
	if execResult != nil && strings.TrimSpace(execResult.Error) != "" {
		return "error"
	}
	return "completed"
}

// buildToolResultContent 统一构造写回模型上下文的 tool 消息正文。
func buildToolResultContent(execResult *tools.ExecuteResult) string {
	if execResult == nil {
		return "执行结果:\n"
	}
	if execResult.Error != "" {
		return fmt.Sprintf("执行结果:\n%s\n错误: %s", execResult.Output, execResult.Error)
	}
	return fmt.Sprintf("执行结果:\n%s", execResult.Output)
}
