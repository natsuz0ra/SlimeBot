package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"corner/backend/internal/tools"
)

const (
	agentMaxIterations   = 10
	agentApprovalTimeout = 120 * time.Second
)

// ApprovalRequest 发送给前端的工具调用审批请求
type ApprovalRequest struct {
	ToolCallID string            `json:"toolCallId"`
	ToolName   string            `json:"toolName"`
	Command    string            `json:"command"`
	Params     map[string]string `json:"params"`
	Preamble   string            `json:"preamble,omitempty"`
}

// ApprovalResponse 前端返回的审批结果
type ApprovalResponse struct {
	ToolCallID string `json:"toolCallId"`
	Approved   bool   `json:"approved"`
}

// ToolCallResult 工具调用结果，推送给前端展示
type ToolCallResult struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Command    string `json:"command"`
	Output     string `json:"output"`
	Error      string `json:"error"`
}

// AgentCallbacks 是 Agent 循环与外部（WebSocket 控制器）交互的回调集合
type AgentCallbacks struct {
	// OnChunk 推送流式文本片段
	OnChunk func(chunk string) error
	// OnToolCallStart 通知前端模型请求调用工具，等待审批
	OnToolCallStart func(req ApprovalRequest) error
	// WaitApproval 阻塞等待前端用户审批结果
	WaitApproval func(ctx context.Context, toolCallID string) (*ApprovalResponse, error)
	// OnToolCallResult 通知前端工具执行结果
	OnToolCallResult func(result ToolCallResult) error
}

// AgentService 封装 Agent 循环逻辑
type AgentService struct {
	openai *OpenAIClient
}

func NewAgentService(openai *OpenAIClient) *AgentService {
	return &AgentService{openai: openai}
}

// BuildToolDefs 从全局工具注册中心生成 OpenAI function call 的工具定义列表。
// 每个工具的每个命令映射为一个 function，名称格式为 {tool}__{command}。
func BuildToolDefs() []ToolDef {
	var defs []ToolDef
	for _, t := range tools.All() {
		for _, cmd := range t.Commands() {
			properties := make(map[string]any)
			var required []string
			for _, p := range cmd.Params {
				prop := map[string]any{
					"type":        "string",
					"description": p.Description,
				}
				if p.Example != "" {
					prop["example"] = p.Example
				}
				properties[p.Name] = prop
				if p.Required {
					required = append(required, p.Name)
				}
			}

			funcName := t.Name() + "__" + cmd.Name
			desc := fmt.Sprintf("[%s] %s", t.Name(), cmd.Description)

			params := map[string]any{
				"type":       "object",
				"properties": properties,
			}
			if len(required) > 0 {
				params["required"] = required
			}

			defs = append(defs, ToolDef{
				Name:        funcName,
				Description: desc,
				Parameters:  params,
			})
		}
	}
	return defs
}

// RunAgentLoop 执行完整的 Agent 循环：
// 1. 调用 LLM（带 tools）
// 2. 如果返回纯文本 -> 通过 onChunk 推送，循环结束
// 3. 如果返回 tool_calls -> 逐个请求审批 -> 执行 -> 结果追加到上下文 -> 回到步骤1
// 返回最终的纯文本回答。
func (a *AgentService) RunAgentLoop(
	ctx context.Context,
	modelConfig ModelRuntimeConfig,
	contextMessages []ChatMessage,
	callbacks AgentCallbacks,
) (string, error) {
	toolDefs := BuildToolDefs()
	messages := make([]ChatMessage, len(contextMessages))
	copy(messages, contextMessages)

	var finalAnswer strings.Builder

	for i := 0; i < agentMaxIterations; i++ {
		log.Printf("agent_iteration iteration=%d messages=%d", i+1, len(messages))

		var chunkBuf strings.Builder
		result, err := a.openai.StreamChatWithTools(ctx, modelConfig, messages, toolDefs, func(chunk string) error {
			chunkBuf.WriteString(chunk)
			return callbacks.OnChunk(chunk)
		})
		if err != nil {
			return "", fmt.Errorf("agent 第 %d 轮 LLM 调用失败: %w", i+1, err)
		}

		if result.Type == StreamResultText {
			finalAnswer.WriteString(chunkBuf.String())
			return finalAnswer.String(), nil
		}

		// tool_calls: 将 assistant 消息（含 tool_calls）追加到上下文
		messages = append(messages, result.AssistantMessage)
		preamble := strings.TrimSpace(result.AssistantMessage.Content)

		for _, tc := range result.ToolCalls {
			toolName, command, err := parseToolCallName(tc.Name)
			if err != nil {
				messages = append(messages, ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("工具调用解析失败: %s", err.Error()),
				})
				continue
			}

			params, err := parseToolCallArgs(tc.Arguments)
			if err != nil {
				messages = append(messages, ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    fmt.Sprintf("参数解析失败: %s", err.Error()),
				})
				continue
			}

			// 通知前端，等待审批
			if err := callbacks.OnToolCallStart(ApprovalRequest{
				ToolCallID: tc.ID,
				ToolName:   toolName,
				Command:    command,
				Params:     params,
				Preamble:   preamble,
			}); err != nil {
				return "", fmt.Errorf("推送工具调用审批请求失败: %w", err)
			}

			approvalCtx, cancel := context.WithTimeout(ctx, agentApprovalTimeout)
			approval, err := callbacks.WaitApproval(approvalCtx, tc.ID)
			cancel()

			if err != nil {
				messages = append(messages, ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    "用户审批超时或发生错误，工具调用已取消。",
				})
				if cbErr := callbacks.OnToolCallResult(ToolCallResult{
					ToolCallID: tc.ID, ToolName: toolName, Command: command,
					Error: "审批超时",
				}); cbErr != nil {
					log.Printf("推送工具结果失败: %v", cbErr)
				}
				continue
			}

			if !approval.Approved {
				messages = append(messages, ChatMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    "用户拒绝了此工具调用，请换一种方式回答或告知用户需要授权才能完成此操作。",
				})
				if cbErr := callbacks.OnToolCallResult(ToolCallResult{
					ToolCallID: tc.ID, ToolName: toolName, Command: command,
					Error: "用户拒绝执行",
				}); cbErr != nil {
					log.Printf("推送工具结果失败: %v", cbErr)
				}
				continue
			}

			// 执行工具
			execResult := executeToolCall(toolName, command, params)

			if cbErr := callbacks.OnToolCallResult(ToolCallResult{
				ToolCallID: tc.ID,
				ToolName:   toolName,
				Command:    command,
				Output:     execResult.Output,
				Error:      execResult.Error,
			}); cbErr != nil {
				log.Printf("推送工具结果失败: %v", cbErr)
			}

			var resultContent string
			if execResult.Error != "" {
				resultContent = fmt.Sprintf("执行结果:\n%s\n错误: %s", execResult.Output, execResult.Error)
			} else {
				resultContent = fmt.Sprintf("执行结果:\n%s", execResult.Output)
			}

			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    resultContent,
			})
		}
	}

	return finalAnswer.String(), fmt.Errorf("agent 循环达到最大迭代次数 (%d)", agentMaxIterations)
}

// parseToolCallName 解析 "{tool}__{command}" 格式的函数名
func parseToolCallName(funcName string) (toolName, command string, err error) {
	parts := strings.SplitN(funcName, "__", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("无效的工具函数名格式: %s", funcName)
	}
	return parts[0], parts[1], nil
}

func parseToolCallArgs(arguments string) (map[string]string, error) {
	if strings.TrimSpace(arguments) == "" {
		return map[string]string{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val
		default:
			b, _ := json.Marshal(val)
			result[k] = string(b)
		}
	}
	return result, nil
}

func executeToolCall(toolName, command string, params map[string]string) *tools.ExecuteResult {
	t, ok := tools.Get(toolName)
	if !ok {
		return &tools.ExecuteResult{Error: fmt.Sprintf("工具 %s 不存在", toolName)}
	}
	result, err := t.Execute(command, params)
	if err != nil {
		return &tools.ExecuteResult{Error: err.Error()}
	}
	return result
}
