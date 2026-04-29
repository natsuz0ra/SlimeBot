package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"sort"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/mcp"
	llmsvc "slimebot/internal/services/llm"
	memsvc "slimebot/internal/services/memory"
	skillsvc "slimebot/internal/services/skill"
	"slimebot/internal/tools"
)

// ApprovalRequest is sent to the client for tool-call approval.
type ApprovalRequest struct {
	ToolCallID       string            `json:"toolCallId"`
	ToolName         string            `json:"toolName"`
	Command          string            `json:"command"`
	Params           map[string]string `json:"params"`
	RequiresApproval bool              `json:"requiresApproval"`
	Preamble         string            `json:"preamble,omitempty"`
	ParentToolCallID string            `json:"parentToolCallId,omitempty"`
	SubagentRunID    string            `json:"subagentRunId,omitempty"`
}

// ApprovalResponse is the client's approval decision.
type ApprovalResponse struct {
	ToolCallID string `json:"toolCallId"`
	Approved   bool   `json:"approved"`
	Answers    string `json:"answers,omitempty"` // JSON-encoded answers for ask_questions tool
}

// ToolCallResult is pushed to the client after tool execution.
type ToolCallResult struct {
	ToolCallID       string `json:"toolCallId"`
	ToolName         string `json:"toolName"`
	Command          string `json:"command"`
	RequiresApproval bool   `json:"requiresApproval"`
	Status           string `json:"status"`
	Output           string `json:"output"`
	Error            string `json:"error"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`
	SubagentRunID    string `json:"subagentRunId,omitempty"`
}

type ThinkingEventMeta struct {
	ParentToolCallID string
	SubagentRunID    string
}

// AgentCallbacks wires the agent loop to the outside world (streaming, approval, results).
type AgentCallbacks struct {
	OnChunk          func(chunk string) error                                                // stream text chunks to the client
	OnToolCallStart  func(req ApprovalRequest) error                                         // notify client that a tool awaits approval
	WaitApproval     func(ctx context.Context, toolCallID string) (*ApprovalResponse, error) // block until approval
	OnToolCallResult func(result ToolCallResult) error                                       // notify client of tool outcome
	OnSubagentStart  func(parentToolCallID, runID, task string) error
	OnSubagentChunk  func(parentToolCallID, runID, chunk string) error
	OnSubagentDone   func(parentToolCallID, runID string, runErr error) error
	OnThinkingStart  func(meta ThinkingEventMeta) error
	OnThinkingChunk  func(chunk string, meta ThinkingEventMeta) error
	OnThinkingDone   func(meta ThinkingEventMeta) error
	OnPlanStart      func() error                  // plan writing phase has begun
	OnPlanChunk      func(chunk string) error      // stream plan body chunk to the client (plan mode only)
	OnPlanBody       func(planBody string) error   // send complete plan body (non-streaming, plan mode only)
	OnTitleGenerated func(sessionID, title string) // async notification when title is generated
}

// AgentLoopOptions configures nested agent execution.
type AgentLoopOptions struct {
	Depth           int
	ApprovalMode    string
	PlanMode        bool
	PlanStarted     *bool  // set to true when plan_start tool is called
	PlanComplete    *bool  // set to true when plan_complete tool is called
	SubagentModelID string // user-selected subagent model override (empty = inherit)
}

// AgentService runs the LLM loop with tools, approvals, and MCP/skill loading.
type AgentService struct {
	providerFactory *llmsvc.Factory
	mcp             *mcp.Manager
	skillRuntime    *skillsvc.SkillRuntimeService
	memory          *memsvc.MemoryService
	subagentHost    SubagentHost
	toolCacheMu     sync.Mutex
	toolCache       map[string]cachedToolDefs
}

// cachedToolDefs is a cached tool-definition bundle with MCP metadata.
type cachedToolDefs struct {
	defs       []llmsvc.ToolDef
	metaByFunc map[string]mcp.ToolMeta
	expireAt   time.Time
}

// NewAgentService constructs an AgentService.
func NewAgentService(providerFactory *llmsvc.Factory, mcpManager *mcp.Manager, skillRuntime *skillsvc.SkillRuntimeService, memory *memsvc.MemoryService) *AgentService {
	return &AgentService{
		providerFactory: providerFactory,
		mcp:             mcpManager,
		skillRuntime:    skillRuntime,
		memory:          memory,
		toolCache:       make(map[string]cachedToolDefs),
	}
}

// SetSubagentHost wires ChatService (or tests) for run_subagent delegation.
func (a *AgentService) SetSubagentHost(h SubagentHost) {
	a.subagentHost = h
}

// BuildToolDefs builds function-calling tool definitions from the global registry.
// Each command becomes one function named {tool}__{command}.
func BuildToolDefs() []llmsvc.ToolDef {
	var defs []llmsvc.ToolDef
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

			defs = append(defs, llmsvc.ToolDef{
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

func buildRunSubagentToolDef() llmsvc.ToolDef {
	return llmsvc.ToolDef{
		Name:        constants.RunSubagentTool,
		Description: "[subagent] Delegate bounded, concise, independent sub-tasks to a nested agent with isolated context (no chat history). Prefer this only when separate focused research, codebase inspection, validation, or summarization has a clear stopping point. The parent agent remains responsible for integrating the result.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Concrete self-contained task for the sub-agent, including the expected deliverable and boundaries.",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional compressed background from the main assistant; include only state the isolated sub-agent needs.",
				},
			},
			"required": []string{"task"},
		},
	}
}

func buildPlanCompleteToolDef() llmsvc.ToolDef {
	return llmsvc.ToolDef{
		Name:        constants.PlanCompleteTool,
		Description: "[plan] Call this tool ONLY when your complete plan has been written in your response. This submits the plan for user review. You MUST call this tool when you finish writing your plan — without it the user will not see the review menu.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Short title for the plan. Omit to auto-detect from the first heading.",
				},
			},
		},
	}
}

func buildPlanStartToolDef() llmsvc.ToolDef {
	return llmsvc.ToolDef{
		Name:        constants.PlanStartTool,
		Description: "[plan] Call this tool when you are ready to begin writing your plan. All text output BEFORE this call will appear as narration; all text AFTER will be the plan body. You MUST call this before writing your plan.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

// buildRuntimeToolDefs merges built-in, skill, and MCP tools and returns MCP name mapping.
func (a *AgentService) buildRuntimeToolDefs(ctx context.Context, configs []domain.MCPConfig, depth int) ([]llmsvc.ToolDef, map[string]mcp.ToolMeta, error) {
	cacheKey := buildToolDefsCacheKey(configs, depth)
	if defs, metaByFunc, ok := a.getCachedToolDefs(cacheKey); ok {
		return defs, metaByFunc, nil
	}
	defs := BuildToolDefs()
	metaByFunc := make(map[string]mcp.ToolMeta)
	if a.memory != nil {
		defs = append(defs, llmsvc.ToolDef{
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
	if depth == 0 {
		defs = append(defs, buildRunSubagentToolDef())
	}
	if a.mcp == nil || len(configs) == 0 {
		return defs, metaByFunc, nil
	}

	loadStart := time.Now()
	metas, mcpDefs, err := a.mcp.LoadTools(ctx, configs)
	logging.Span("mcp_load_tools", loadStart)
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
		defs = append(defs, llmsvc.ToolDef{
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
			logging.Warn("tool_name_too_long", "name", def.Name, "len", nameLen)
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

func buildToolDefsCacheKey(configs []domain.MCPConfig, depth int) string {
	base := "none"
	if len(configs) > 0 {
		parts := make([]string, 0, len(configs))
		for _, item := range configs {
			parts = append(parts, item.ID+":"+item.UpdatedAt.UTC().Format(time.RFC3339Nano))
		}
		base = strings.Join(parts, "|")
	}
	return fmt.Sprintf("%s|d%d", base, depth)
}

func (a *AgentService) getCachedToolDefs(cacheKey string) ([]llmsvc.ToolDef, map[string]mcp.ToolMeta, bool) {
	a.toolCacheMu.Lock()
	defer a.toolCacheMu.Unlock()
	item, ok := a.toolCache[cacheKey]
	if !ok || time.Now().After(item.expireAt) {
		return nil, nil, false
	}
	defs := make([]llmsvc.ToolDef, len(item.defs))
	copy(defs, item.defs)
	metaByFunc := make(map[string]mcp.ToolMeta, len(item.metaByFunc))
	for k, v := range item.metaByFunc {
		metaByFunc[k] = v
	}
	return defs, metaByFunc, true
}

func (a *AgentService) setCachedToolDefs(cacheKey string, defs []llmsvc.ToolDef, metaByFunc map[string]mcp.ToolMeta) {
	a.toolCacheMu.Lock()
	defer a.toolCacheMu.Unlock()
	defsCopy := make([]llmsvc.ToolDef, len(defs))
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

// RunAgentLoop runs the full agent loop:
// 1) Call the LLM with tools.
// 2) If text only, stream via OnChunk and return.
// 3) If tool_calls, approve each, execute, append tool results, repeat from step 1.
// Returns the final assistant text answer.
func (a *AgentService) RunAgentLoop(
	ctx context.Context,
	modelConfig llmsvc.ModelRuntimeConfig,
	sessionID string,
	contextMessages []llmsvc.ChatMessage,
	mcpConfigs []domain.MCPConfig,
	activatedSkills map[string]struct{},
	callbacks AgentCallbacks,
	opts AgentLoopOptions,
) (string, error) {
	toolDefs, mcpToolMeta, err := a.buildRuntimeToolDefs(ctx, mcpConfigs, opts.Depth)
	if err != nil {
		return "", fmt.Errorf("failed to load MCP tools: %w", err)
	}

	// Plan mode: only expose read-only tools so the model cannot even attempt to call others.
	if opts.PlanMode {
		toolDefs = filterPlanModeToolDefs(toolDefs)
		mcpToolMeta = filterPlanModeMCPMeta(mcpToolMeta)
		toolDefs = append(toolDefs, buildPlanStartToolDef())
		toolDefs = append(toolDefs, buildPlanCompleteToolDef())
	}
	messages := make([]llmsvc.ChatMessage, len(contextMessages))
	copy(messages, contextMessages)

	var finalAnswer strings.Builder
	memoryToolUsed := false

	provider := a.providerFactory.GetProvider(modelConfig.Provider)

	for i := 0; ; i++ {
		if opts.Depth == 0 && i >= constants.AgentMaxIterations {
			return finalAnswer.String(), fmt.Errorf("agent loop reached max iterations (%d)", constants.AgentMaxIterations)
		}

		logging.Info("agent_iteration", "iteration", i+1, "messages", len(messages), "agent_depth", opts.Depth)

		var chunkBuf strings.Builder
		var thinkingStarted bool
		var thinkingDone bool
		thinkingMeta := ThinkingEventMeta{}
		finishThinking := func() error {
			if !thinkingStarted || thinkingDone || callbacks.OnThinkingDone == nil {
				thinkingDone = thinkingStarted
				return nil
			}
			if err := callbacks.OnThinkingDone(thinkingMeta); err != nil {
				return err
			}
			thinkingDone = true
			return nil
		}
		result, err := provider.StreamChatWithTools(ctx, modelConfig, messages, toolDefs, llmsvc.StreamCallbacks{
			OnChunk: func(chunk string) error {
				if chunk != "" {
					if err := finishThinking(); err != nil {
						return err
					}
				}
				chunkBuf.WriteString(chunk)
				if callbacks.OnChunk == nil {
					return nil
				}
				return callbacks.OnChunk(chunk)
			},
			OnThinkingChunk: func(thinkingChunk string) error {
				if !thinkingStarted {
					thinkingStarted = true
					if callbacks.OnThinkingStart != nil {
						if err := callbacks.OnThinkingStart(thinkingMeta); err != nil {
							return err
						}
					}
				}
				if callbacks.OnThinkingChunk == nil {
					return nil
				}
				return callbacks.OnThinkingChunk(thinkingChunk, thinkingMeta)
			},
		})
		if err != nil {
			return "", fmt.Errorf("agent LLM call failed at iteration %d: %w", i+1, err)
		}

		if thinkingStarted && !thinkingDone {
			if err := finishThinking(); err != nil {
				return "", fmt.Errorf("OnThinkingDone callback failed: %w", err)
			}
		}

		if result.Type == llmsvc.StreamResultText {
			finalAnswer.WriteString(chunkBuf.String())
			return finalAnswer.String(), nil
		}

		// tool_calls: append assistant message (with tool_calls) to context.
		messages = append(messages, result.AssistantMessage)
		//preamble := strings.TrimSpace(result.AssistantMessage.Content)

		for _, tc := range result.ToolCalls {
			// Handle plan_start: signal transition from research to plan writing.
			if tc.Name == constants.PlanStartTool {
				if opts.PlanStarted != nil {
					*opts.PlanStarted = true
				}
				if callbacks.OnPlanStart != nil {
					if err := callbacks.OnPlanStart(); err != nil {
						return "", fmt.Errorf("OnPlanStart callback failed: %w", err)
					}
				}
				messages = appendToolMessage(messages, tc.ID, "Plan writing phase started.")
				continue
			}

			// Handle plan_complete: signal plan completion and skip regular execution.
			if tc.Name == constants.PlanCompleteTool {
				if opts.PlanComplete != nil {
					*opts.PlanComplete = true
				}
				messages = appendToolMessage(messages, tc.ID, "Plan submitted for review.")
				continue
			}

			// Plan mode: block non-read-only tools.
			if opts.PlanMode && !isPlanModeAllowedTool(tc.Name) {
				messages = appendToolMessage(messages, tc.ID, "This tool is blocked in plan mode. Only read-only tools (web_search, search_memory) are allowed.")
				continue
			}

			invocation, err := resolveToolInvocation(tc, mcpToolMeta, opts.ApprovalMode)
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

			if tc.Name == constants.RunSubagentTool {
				if err := a.handleRunSubagentTool(ctx, modelConfig, sessionID, mcpConfigs, activatedSkills, callbacks, opts, tc, invocation, params, opts.SubagentModelID, "", &messages); err != nil {
					return "", err
				}
				continue
			}

			if callbacks.OnToolCallStart != nil {
				if err := callbacks.OnToolCallStart(ApprovalRequest{
					ToolCallID:       tc.ID,
					ToolName:         invocation.toolName,
					Command:          invocation.command,
					Params:           params,
					RequiresApproval: invocation.requiresApproval,
				}); err != nil {
					return "", fmt.Errorf("failed to push tool approval request: %w", err)
				}
			}

			approved, rejectionMessage, answers := waitApprovalIfNeeded(ctx, callbacks, tc, invocation, params, "")
			if !approved {
				messages = appendToolMessage(messages, tc.ID, rejectionMessage)
				continue
			}

			// ask_questions tool: use answers from approval directly, skip executeInvocation.
			if invocation.toolName == constants.AskQuestionsTool {
				formattedAnswers := formatAskQuestionsAnswers(params["questions"], answers)
				notifyToolResult(callbacks, ToolCallResult{
					ToolCallID:       tc.ID,
					ToolName:         invocation.toolName,
					Command:          invocation.command,
					RequiresApproval: invocation.requiresApproval,
					Status:           constants.ToolCallStatusCompleted,
					Output:           formattedAnswers,
				})
				messages = appendToolMessage(messages, tc.ID, "User answers:\n"+formattedAnswers)
				continue
			}

			// Execute tool.
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

		// If plan_complete was called, return immediately so the caller can save the plan.
		if opts.PlanComplete != nil && *opts.PlanComplete {
			return finalAnswer.String(), nil
		}
	}

	return finalAnswer.String(), nil
}

// isPlanModeAllowedTool returns true if the tool function name is allowed in plan mode.
func isPlanModeAllowedTool(funcName string) bool {
	// Handle tools without __ separator (e.g. search_memory, plan_start).
	switch funcName {
	case "search_memory", "plan_start", constants.RunSubagentTool:
		return true
	}
	// Handle tools with __ separator (e.g. web_search__search, plan_complete__submit).
	toolName, _, _ := parseToolCallName(funcName)
	switch toolName {
	case "web_search", "plan_complete":
		return true
	default:
		return false
	}
}

// filterPlanModeToolDefs keeps only read-only tool definitions for plan mode.
func filterPlanModeToolDefs(defs []llmsvc.ToolDef) []llmsvc.ToolDef {
	var filtered []llmsvc.ToolDef
	for _, d := range defs {
		if isPlanModeAllowedTool(d.Name) {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// filterPlanModeMCPMeta keeps only MCP metadata entries for read-only tools.
func filterPlanModeMCPMeta(meta map[string]mcp.ToolMeta) map[string]mcp.ToolMeta {
	filtered := make(map[string]mcp.ToolMeta)
	for k, v := range meta {
		toolName, _, err := parseToolCallName(k)
		if err != nil {
			continue
		}
		if isPlanModeAllowedTool(toolName) {
			filtered[k] = v
		}
	}
	return filtered
}

// parseToolCallName parses "{tool}__{command}" function names.
func parseToolCallName(funcName string) (toolName, command string, err error) {
	parts := strings.SplitN(funcName, "__", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid tool function name format: %s", funcName)
	}
	return parts[0], parts[1], nil
}

// parseToolCallArgs normalizes tool arguments to string maps for built-in tools.
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

// parseToolCallArgsAny preserves raw JSON types for MCP tool calls.
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

// executeToolCall runs a built-in tool command with uniform error handling.
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

// requiresToolApproval defines which tools need user approval.
// When approvalMode is "auto", all tools skip approval except ask_questions (which always needs user interaction).
func requiresToolApproval(toolName string, isMCP bool, approvalMode string) bool {
	if toolName == constants.AskQuestionsTool {
		return true
	}
	if approvalMode == constants.ApprovalModeAuto {
		return false
	}
	if isMCP {
		return false
	}
	return toolName == constants.ExecToolName
}

func appendToolMessage(messages []llmsvc.ChatMessage, toolCallID string, content string) []llmsvc.ChatMessage {
	return append(messages, llmsvc.ChatMessage{
		Role:       "tool",
		ToolCallID: toolCallID,
		Content:    content,
	})
}

func formatAskQuestionsAnswers(questionsJSON string, answersJSON string) string {
	type qItem struct {
		ID       string   `json:"id"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	type answer struct {
		QuestionID     string `json:"questionId"`
		SelectedOption int    `json:"selectedOption"`
		CustomAnswer   string `json:"customAnswer"`
	}
	type readableAnswer struct {
		ID       string `json:"id"`
		Question string `json:"question"`
		Answer   string `json:"answer"`
	}

	var questions []qItem
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return answersJSON
	}
	var answers []answer
	if err := json.Unmarshal([]byte(answersJSON), &answers); err != nil {
		return answersJSON
	}

	qMap := make(map[string]qItem, len(questions))
	for _, q := range questions {
		qMap[q.ID] = q
	}

	result := make([]readableAnswer, 0, len(answers))
	for _, a := range answers {
		q, ok := qMap[a.QuestionID]
		if !ok {
			result = append(result, readableAnswer{ID: a.QuestionID, Question: "(unknown)", Answer: a.CustomAnswer})
			continue
		}
		ansText := a.CustomAnswer
		if a.SelectedOption >= 0 && a.SelectedOption < len(q.Options) {
			ansText = q.Options[a.SelectedOption]
		}
		result = append(result, readableAnswer{ID: q.ID, Question: q.Question, Answer: ansText})
	}

	b, err := json.Marshal(result)
	if err != nil {
		return answersJSON
	}
	return string(b)
}
