package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slimebot/internal/domain"
	"slimebot/internal/observability"
	"sort"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/mcp"
	memsvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	skillsvc "slimebot/internal/services/skill"
	"slimebot/internal/tools"
)

const balancedTemperature = 1.2

// ApprovalRequest 发送给前端的工具调用审批请求
type ApprovalRequest struct {
	ToolCallID       string            `json:"toolCallId"`
	ToolName         string            `json:"toolName"`
	Command          string            `json:"command"`
	Params           map[string]string `json:"params"`
	RequiresApproval bool              `json:"requiresApproval"`
	Preamble         string            `json:"preamble,omitempty"`
}

// ApprovalResponse 前端返回的审批结果
type ApprovalResponse struct {
	ToolCallID string `json:"toolCallId"`
	Approved   bool   `json:"approved"`
}

// ToolCallResult 工具调用结果，推送给前端展示
type ToolCallResult struct {
	ToolCallID       string `json:"toolCallId"`
	ToolName         string `json:"toolName"`
	Command          string `json:"command"`
	RequiresApproval bool   `json:"requiresApproval"`
	Status           string `json:"status"`
	Output           string `json:"output"`
	Error            string `json:"error"`
}

// AgentCallbacks Agent 循环与外部交互的回调集合
type AgentCallbacks struct {
	OnChunk          func(chunk string) error                                                // 推送流式文本片段
	OnToolCallStart  func(req ApprovalRequest) error                                         // 通知前端工具调用等待审批
	WaitApproval     func(ctx context.Context, toolCallID string) (*ApprovalResponse, error) // 阻塞等待审批结果
	OnToolCallResult func(result ToolCallResult) error                                       // 通知前端工具执行结果
}

// AgentService Agent 服务：封装与 LLM 的交互循环、工具调用、审批流与 MCP/Skill 工具加载
type AgentService struct {
	openai       *oaisvc.OpenAIClient
	mcp          *mcp.Manager
	skillRuntime *skillsvc.SkillRuntimeService
	memory       *memsvc.MemoryService
	toolCacheMu  sync.Mutex
	toolCache    map[string]cachedToolDefs
}

// cachedToolDefs 工具定义缓存项
type cachedToolDefs struct {
	defs       []oaisvc.ToolDef
	metaByFunc map[string]mcp.ToolMeta
	expireAt   time.Time
}

// NewAgentService 创建 Agent 服务实例
func NewAgentService(openai *oaisvc.OpenAIClient, mcpManager *mcp.Manager, skillRuntime *skillsvc.SkillRuntimeService, memory *memsvc.MemoryService) *AgentService {
	return &AgentService{
		openai:       openai,
		mcp:          mcpManager,
		skillRuntime: skillRuntime,
		memory:       memory,
		toolCache:    make(map[string]cachedToolDefs),
	}
}

// BuildToolDefs 从全局工具注册中心生成 OpenAI function call 的工具定义列表
// 每个工具的每个命令映射为一个 function，名称格式为 {tool}__{command}
func BuildToolDefs() []oaisvc.ToolDef {
	var defs []oaisvc.ToolDef
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

			defs = append(defs, oaisvc.ToolDef{
				Name:        funcName,
				Description: desc,
				Parameters:  params,
			})
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Name == defs[j].Name {
			return defs[i].Description < defs[j].Description
		}
		return defs[i].Name < defs[j].Name
	})
	return defs
}

// buildRuntimeToolDefs 汇总运行时可见工具（内建 + skill + MCP）并返回名称映射
func (a *AgentService) buildRuntimeToolDefs(ctx context.Context, configs []domain.MCPConfig) ([]oaisvc.ToolDef, map[string]mcp.ToolMeta, error) {
	cacheKey := buildToolDefsCacheKey(configs)
	if defs, metaByFunc, ok := a.getCachedToolDefs(cacheKey); ok {
		return defs, metaByFunc, nil
	}
	defs := BuildToolDefs()
	metaByFunc := make(map[string]mcp.ToolMeta)
	if a.memory != nil {
		// 注入 memory 工具定义，供模型在需要时检索历史记忆
		defs = append(defs, oaisvc.ToolDef{
			Name:        constants.SearchMemoryTool,
			Description: "[memory] Search historical memory on demand. Use only when the response depends on past preferences, decisions, or cross-session constraints.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Memory topic or question to retrieve for this turn.",
					},
					"top_k": map[string]any{
						"type":        "integer",
						"description": "Number of results to return, default 3, max 5.",
						"default":     constants.MemoryToolDefaultTopK,
						"minimum":     1,
						"maximum":     5,
					},
				},
				"required": []string{"query"},
			},
		})
	}
	if a.skillRuntime != nil {
		skills, err := a.skillRuntime.ListSkills()
		if err != nil {
			return nil, nil, err
		}
		if def := a.skillRuntime.BuildActivateSkillToolDef(skills); def != nil {
			defs = append(defs, *def)
		}
	}
	if a.mcp == nil || len(configs) == 0 {
		return defs, metaByFunc, nil
	}

	loadStart := time.Now()
	metas, mcpDefs, err := a.mcp.LoadTools(ctx, configs)
	observability.Span("mcp_load_tools", loadStart)
	if err != nil {
		return nil, nil, err
	}
	for _, def := range mcpDefs {
		name, _ := def["name"].(string)
		description, _ := def["description"].(string)
		parameters, _ := def["parameters"].(map[string]any)
		if name == "" {
			continue
		}
		defs = append(defs, oaisvc.ToolDef{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		})
	}
	for _, meta := range metas {
		metaByFunc[meta.FuncName] = meta
	}
	for _, def := range defs {
		nameLen := len(def.Name)
		if nameLen > constants.MaxToolNameLen {
			slog.Warn("tool_name_too_long", "name", def.Name, "len", nameLen)
			return nil, nil, fmt.Errorf("tool name is too long: %s (len=%d, max=%d)", def.Name, nameLen, constants.MaxToolNameLen)
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		if defs[i].Name == defs[j].Name {
			return defs[i].Description < defs[j].Description
		}
		return defs[i].Name < defs[j].Name
	})
	a.setCachedToolDefs(cacheKey, defs, metaByFunc)
	return defs, metaByFunc, nil
}

func buildToolDefsCacheKey(configs []domain.MCPConfig) string {
	if len(configs) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(configs))
	for _, item := range configs {
		parts = append(parts, item.ID+":"+item.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	return strings.Join(parts, "|")
}

func (a *AgentService) getCachedToolDefs(cacheKey string) ([]oaisvc.ToolDef, map[string]mcp.ToolMeta, bool) {
	a.toolCacheMu.Lock()
	defer a.toolCacheMu.Unlock()
	item, ok := a.toolCache[cacheKey]
	if !ok || time.Now().After(item.expireAt) {
		return nil, nil, false
	}
	defs := make([]oaisvc.ToolDef, len(item.defs))
	copy(defs, item.defs)
	metaByFunc := make(map[string]mcp.ToolMeta, len(item.metaByFunc))
	for k, v := range item.metaByFunc {
		metaByFunc[k] = v
	}
	return defs, metaByFunc, true
}

func (a *AgentService) setCachedToolDefs(cacheKey string, defs []oaisvc.ToolDef, metaByFunc map[string]mcp.ToolMeta) {
	a.toolCacheMu.Lock()
	defer a.toolCacheMu.Unlock()
	defsCopy := make([]oaisvc.ToolDef, len(defs))
	copy(defsCopy, defs)
	metaCopy := make(map[string]mcp.ToolMeta, len(metaByFunc))
	for k, v := range metaByFunc {
		metaCopy[k] = v
	}
	a.toolCache[cacheKey] = cachedToolDefs{
		defs:       defsCopy,
		metaByFunc: metaCopy,
		expireAt:   time.Now().Add(10 * time.Minute),
	}
}

// RunAgentLoop 执行完整的 Agent 循环：
// 1. 调用 LLM（带 tools）
// 2. 如果返回纯文本 -> 通过 onChunk 推送，循环结束
// 3. 如果返回 tool_calls -> 逐个请求审批 -> 执行 -> 结果追加到上下文 -> 回到步骤1
// 返回最终的纯文本回答。
func (a *AgentService) RunAgentLoop(
	ctx context.Context,
	modelConfig oaisvc.ModelRuntimeConfig,
	sessionID string,
	contextMessages []oaisvc.ChatMessage,
	mcpConfigs []domain.MCPConfig,
	activatedSkills map[string]struct{},
	callbacks AgentCallbacks,
) (string, error) {
	modelConfig.Temperature = balancedTemperature

	toolDefs, mcpToolMeta, err := a.buildRuntimeToolDefs(ctx, mcpConfigs)
	if err != nil {
		return "", fmt.Errorf("failed to load MCP tools: %w", err)
	}
	messages := make([]oaisvc.ChatMessage, len(contextMessages))
	copy(messages, contextMessages)

	var finalAnswer strings.Builder
	memoryToolUsed := false

	for i := 0; i < constants.AgentMaxIterations; i++ {
		slog.Info("agent_iteration", "iteration", i+1, "messages", len(messages))

		var chunkBuf strings.Builder
		result, err := a.openai.StreamChatWithTools(ctx, modelConfig, messages, toolDefs, func(chunk string) error {
			chunkBuf.WriteString(chunk)
			return callbacks.OnChunk(chunk)
		})
		if err != nil {
			return "", fmt.Errorf("agent LLM call failed at iteration %d: %w", i+1, err)
		}

		if result.Type == oaisvc.StreamResultType(constants.StreamResultText) {
			finalAnswer.WriteString(chunkBuf.String())
			return finalAnswer.String(), nil
		}

		// tool_calls: 将 assistant 消息（含 tool_calls）追加到上下文
		messages = append(messages, result.AssistantMessage)
		preamble := strings.TrimSpace(result.AssistantMessage.Content)

		for _, tc := range result.ToolCalls {
			// tool_calls 阶段会逐个审批并执行（含 memory 工具）。
			invocation, err := resolveToolInvocation(tc, mcpToolMeta)
			if err != nil {
				messages = appendToolMessage(messages, tc.ID, fmt.Sprintf("failed to parse tool invocation: %s", err.Error()))
				continue
			}

			params, err := parseToolCallArgs(tc.Arguments)
			if err != nil {
				messages = appendToolMessage(messages, tc.ID, fmt.Sprintf("failed to parse arguments: %s", err.Error()))
				continue
			}

			if tc.Name == constants.ActivateSkillTool && a.skillRuntime != nil {
				skillName := strings.TrimSpace(params["name"])
				content, _, activateErr := a.skillRuntime.ActivateSkill(skillName, activatedSkills)
				if activateErr != nil {
					messages = appendToolMessage(messages, tc.ID, fmt.Sprintf("failed to activate skill: %s", activateErr.Error()))
					continue
				}
				messages = appendToolMessage(messages, tc.ID, content)
				continue
			}

			if err := callbacks.OnToolCallStart(ApprovalRequest{
				ToolCallID:       tc.ID,
				ToolName:         invocation.toolName,
				Command:          invocation.command,
				Params:           params,
				RequiresApproval: invocation.requiresApproval,
				Preamble:         preamble,
			}); err != nil {
				return "", fmt.Errorf("failed to push tool approval request: %w", err)
			}

			approved, rejectionMessage := waitApprovalIfNeeded(ctx, callbacks, tc, invocation, params, preamble)
			if !approved {
				messages = appendToolMessage(messages, tc.ID, rejectionMessage)
				continue
			}

			// 执行工具
			execResult := a.executeInvocation(ctx, tc, invocation, params, sessionID, mcpConfigs, &memoryToolUsed)
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

			messages = appendToolMessage(messages, tc.ID, buildToolResultContent(execResult))
		}
	}

	return finalAnswer.String(), fmt.Errorf("agent loop reached max iterations (%d)", constants.AgentMaxIterations)
}

// parseToolCallName 解析 "{tool}__{command}" 格式的函数名
func parseToolCallName(funcName string) (toolName, command string, err error) {
	parts := strings.SplitN(funcName, "__", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid tool function name format: %s", funcName)
	}
	return parts[0], parts[1], nil
}

// parseToolCallArgs 将工具参数统一转换为 string map，供内建工具执行层使用。
// 非字符串值会转成紧凑 JSON 文本，保证参数信息不丢失。
func parseToolCallArgs(arguments string) (map[string]string, error) {
	if strings.TrimSpace(arguments) == "" {
		return map[string]string{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
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

// parseToolCallArgsAny 保留参数原始类型，用于 MCP 工具调用。
func parseToolCallArgsAny(arguments string) (map[string]any, error) {
	if strings.TrimSpace(arguments) == "" {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return raw, nil
}

// executeToolCall 执行内建工具命令并统一错误返回格式。
func executeToolCall(ctx context.Context, toolName, command string, params map[string]string) *tools.ExecuteResult {
	t, ok := tools.Get(toolName)
	if !ok {
		return &tools.ExecuteResult{Error: fmt.Sprintf("tool %s not found", toolName)}
	}
	result, err := t.Execute(ctx, command, params)
	if err != nil {
		return &tools.ExecuteResult{Error: err.Error()}
	}
	return result
}

// requiresToolApproval 定义工具审批策略（当前仅 exec 需要审批）。
func requiresToolApproval(toolName string, isMCP bool) bool {
	if isMCP {
		return false
	}
	return toolName == constants.ExecToolName
}

func appendToolMessage(messages []oaisvc.ChatMessage, toolCallID string, content string) []oaisvc.ChatMessage {
	return append(messages, oaisvc.ChatMessage{
		Role:       "tool",
		ToolCallID: toolCallID,
		Content:    content,
	})
}
