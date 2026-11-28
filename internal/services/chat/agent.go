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
	sandboxpolicy "slimebot/internal/sandbox"
	contextsvc "slimebot/internal/services/context"
	llmsvc "slimebot/internal/services/llm"
	memorysvc "slimebot/internal/services/memory"
	schedulesvc "slimebot/internal/services/schedule"
	skillsvc "slimebot/internal/services/skill"
	usagesvc "slimebot/internal/services/usage"
	"slimebot/internal/tools"
)

const todoUpdateFuncName = tools.TodoUpdateFunctionName

// AgentService runs the LLM loop with tools, approvals, and MCP/skill loading.
type AgentService struct {
	providerFactory *llmsvc.Factory
	mcp             *mcp.Manager
	memory          *memorysvc.Service
	schedule        *schedulesvc.Service
	skillRuntime    *skillsvc.SkillRuntimeService
	subagentHost    SubagentHost
	toolCacheMu     sync.Mutex
	toolCache       map[string]cachedToolDefs
	readFilesMu     sync.Mutex
	readFilesBySess map[string]*tools.ReadFileState
	readFilesAt     map[string]time.Time
	todosBySess     map[string]*tools.TodoState
	todosAt         map[string]time.Time
	processesBySess map[string]*tools.ProcessManager
	processesAt     map[string]time.Time
}

// cachedToolDefs is a cached tool-definition bundle with MCP metadata.
type cachedToolDefs struct {
	defs       []llmsvc.ToolDef
	metaByFunc map[string]mcp.ToolMeta
	expireAt   time.Time
}

// NewAgentService constructs an AgentService.
func NewAgentService(providerFactory *llmsvc.Factory, mcpManager *mcp.Manager, skillRuntime *skillsvc.SkillRuntimeService) *AgentService {
	return &AgentService{
		providerFactory: providerFactory,
		mcp:             mcpManager,
		skillRuntime:    skillRuntime,
		toolCache:       make(map[string]cachedToolDefs),
		readFilesBySess: make(map[string]*tools.ReadFileState),
		readFilesAt:     make(map[string]time.Time),
		todosBySess:     make(map[string]*tools.TodoState),
		todosAt:         make(map[string]time.Time),
		processesBySess: make(map[string]*tools.ProcessManager),
		processesAt:     make(map[string]time.Time),
	}
}

// SetSubagentHost wires ChatService (or tests) for run_subagent delegation.
func (a *AgentService) SetSubagentHost(h SubagentHost) {
	a.subagentHost = h
}

func (a *AgentService) SetMemoryService(service *memorysvc.Service) {
	a.memory = service
	a.toolCacheMu.Lock()
	a.toolCache = make(map[string]cachedToolDefs)
	a.toolCacheMu.Unlock()
}

func (a *AgentService) SetScheduleService(service *schedulesvc.Service) {
	a.schedule = service
	a.toolCacheMu.Lock()
	a.toolCache = make(map[string]cachedToolDefs)
	a.toolCacheMu.Unlock()
}

func (a *AgentService) getSessionReadFileState(sessionID string) *tools.ReadFileState {
	key := strings.TrimSpace(sessionID)
	if key == "" {
		return tools.NewReadFileState()
	}
	a.readFilesMu.Lock()
	defer a.readFilesMu.Unlock()
	state := a.readFilesBySess[key]
	if state == nil {
		state = tools.NewReadFileState()
		a.readFilesBySess[key] = state
	}
	a.readFilesAt[key] = time.Now()
	if len(a.readFilesBySess) > 1024 {
		a.evictOldReadFileSessionsLocked(256)
	}
	return state
}

func (a *AgentService) evictOldReadFileSessionsLocked(maxEvict int) {
	for i := 0; i < maxEvict && len(a.readFilesAt) > 0; i++ {
		var oldestSession string
		var oldestTime time.Time
		for sessionID, touchedAt := range a.readFilesAt {
			if oldestSession == "" || touchedAt.Before(oldestTime) {
				oldestSession = sessionID
				oldestTime = touchedAt
			}
		}
		if oldestSession == "" {
			return
		}
		delete(a.readFilesBySess, oldestSession)
		delete(a.readFilesAt, oldestSession)
	}
}

func (a *AgentService) getSessionTodoState(sessionID string) *tools.TodoState {
	key := strings.TrimSpace(sessionID)
	if key == "" {
		return tools.NewTodoState()
	}
	a.readFilesMu.Lock()
	defer a.readFilesMu.Unlock()
	state := a.todosBySess[key]
	if state == nil {
		state = tools.NewTodoState()
		a.todosBySess[key] = state
	}
	a.todosAt[key] = time.Now()
	if len(a.todosBySess) > 1024 {
		a.evictOldTodoSessionsLocked(256)
	}
	return state
}

func (a *AgentService) evictOldTodoSessionsLocked(maxEvict int) {
	for i := 0; i < maxEvict && len(a.todosAt) > 0; i++ {
		var oldestSession string
		var oldestTime time.Time
		for sessionID, touchedAt := range a.todosAt {
			if oldestSession == "" || touchedAt.Before(oldestTime) {
				oldestSession = sessionID
				oldestTime = touchedAt
			}
		}
		delete(a.todosBySess, oldestSession)
		delete(a.todosAt, oldestSession)
	}
}

func (a *AgentService) getSessionProcessManager(sessionID string) *tools.ProcessManager {
	key := strings.TrimSpace(sessionID)
	if key == "" {
		return tools.NewProcessManager()
	}
	a.readFilesMu.Lock()
	defer a.readFilesMu.Unlock()
	manager := a.processesBySess[key]
	if manager == nil {
		manager = tools.NewProcessManager()
		a.processesBySess[key] = manager
	}
	a.processesAt[key] = time.Now()
	if len(a.processesBySess) > 1024 {
		a.evictOldProcessSessionsLocked(256)
	}
	return manager
}

func (a *AgentService) evictOldProcessSessionsLocked(maxEvict int) {
	for i := 0; i < maxEvict && len(a.processesAt) > 0; i++ {
		var oldestSession string
		var oldestTime time.Time
		for sessionID, touchedAt := range a.processesAt {
			if manager := a.processesBySess[sessionID]; manager != nil && manager.HasRunning() {
				continue
			}
			if oldestSession == "" || touchedAt.Before(oldestTime) {
				oldestSession = sessionID
				oldestTime = touchedAt
			}
		}
		if oldestSession == "" {
			return
		}
		delete(a.processesBySess, oldestSession)
		delete(a.processesAt, oldestSession)
	}
}

// BuildToolDefs builds function-calling tool definitions from the global registry.
// Each command becomes one function named {tool}__{command}, except stable aliases.
func BuildToolDefs() []llmsvc.ToolDef {
	return tools.BuildRegistryToolDefs()
}

// buildRuntimeToolDefs merges built-in, skill, and MCP tools and returns MCP name mapping.
func (a *AgentService) buildRuntimeToolDefs(ctx context.Context, configs []domain.MCPConfig, depth int) ([]llmsvc.ToolDef, map[string]mcp.ToolMeta, error) {
	cacheKey := buildToolDefsCacheKey(configs, depth)
	surface := constants.ClientSurfaceFromContext(ctx)
	if surface != "" {
		cacheKey += "|surface:" + surface
	}
	if a.skillRuntime != nil {
		cacheKey += "|s:" + a.skillRuntime.ToolCacheKey()
	}
	memoryToolsEnabled := a.memory != nil && a.memory.ToolsEnabled(ctx)
	if a.memory != nil {
		cacheKey += "|" + a.memory.ToolCacheKey(ctx)
	} else {
		cacheKey += "|memory:nil"
	}
	if defs, metaByFunc, ok := a.getCachedToolDefs(cacheKey); ok {
		return defs, metaByFunc, nil
	}
	defs := BuildToolDefs()
	if surface == constants.ClientSurfaceWeb {
		defs = requireExplicitPathForWebFileSearch(defs)
	}
	if !memoryToolsEnabled {
		defs = filterToolDefsByToolName(defs, constants.MemoryToolName)
	}
	metaByFunc := make(map[string]mcp.ToolMeta)
	specialOpts := tools.SpecialToolOptions{IncludeRunSubagent: depth == 0}
	if a.skillRuntime != nil {
		skills, err := a.skillRuntime.ListSkills()
		if err != nil {
			return nil, nil, err
		}
		specialOpts.Skills = skills
	}
	defs = append(defs, tools.BuildSpecialToolDefs(specialOpts)...)
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

func requireExplicitPathForWebFileSearch(defs []llmsvc.ToolDef) []llmsvc.ToolDef {
	out := append([]llmsvc.ToolDef(nil), defs...)
	for i := range out {
		switch out[i].Name {
		case "grep__search":
			out[i] = cloneToolDef(out[i])
			out[i].Description += " In web server mode, there is no user working directory, so path is required."
			markToolParamRequired(&out[i], "path")
			updateToolParamDescription(&out[i], "path", "File or directory to search. Required in web server mode because there is no user working directory.")
		case "glob__find":
			out[i] = cloneToolDef(out[i])
			out[i].Description += " In web server mode, there is no user working directory, so path is required."
			markToolParamRequired(&out[i], "path")
			updateToolParamDescription(&out[i], "path", "Directory to search. Required in web server mode because there is no user working directory.")
		}
	}
	return out
}

func cloneToolDef(def llmsvc.ToolDef) llmsvc.ToolDef {
	params := make(map[string]any, len(def.Parameters))
	for key, value := range def.Parameters {
		if nested, ok := value.(map[string]any); ok {
			copyNested := make(map[string]any, len(nested))
			for nestedKey, nestedValue := range nested {
				copyNested[nestedKey] = nestedValue
			}
			params[key] = copyNested
			continue
		}
		if nested, ok := value.([]string); ok {
			params[key] = append([]string(nil), nested...)
			continue
		}
		params[key] = value
	}
	def.Parameters = params
	return def
}

func markToolParamRequired(def *llmsvc.ToolDef, param string) {
	if def == nil || def.Parameters == nil || strings.TrimSpace(param) == "" {
		return
	}
	required, _ := def.Parameters["required"].([]string)
	if stringSliceContains(required, param) {
		return
	}
	def.Parameters["required"] = append(required, param)
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func updateToolParamDescription(def *llmsvc.ToolDef, param, description string) {
	if def == nil || def.Parameters == nil {
		return
	}
	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		return
	}
	prop, ok := props[param].(map[string]any)
	if !ok {
		return
	}
	prop["description"] = description
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
	var configErr error
	modelConfig, configErr = llmsvc.ResolveRequestConfig(modelConfig)
	if configErr != nil {
		return "", configErr
	}
	scope := llmsvc.PrivacyScopeFromContext(ctx)
	if scope.SessionID != sessionID {
		ctx = llmsvc.WithPrivacyScope(ctx, sessionID, scope.RequestID, "agent")
	}
	toolDefs, mcpToolMeta, err := a.buildRuntimeToolDefs(ctx, mcpConfigs, opts.Depth)
	if err != nil {
		return "", fmt.Errorf("failed to load MCP tools: %w", err)
	}

	// Plan mode: only expose read-only tools so the model cannot even attempt to call others.
	if opts.PlanMode {
		toolDefs = filterPlanModeToolDefs(toolDefs)
		mcpToolMeta = filterPlanModeMCPMeta(mcpToolMeta)
		toolDefs = append(toolDefs, tools.BuildSpecialToolDefs(tools.SpecialToolOptions{IncludePlanTools: true})...)
	}
	if len(opts.AllowedToolFunctions) > 0 {
		toolDefs = filterAllowedToolDefs(toolDefs, opts.AllowedToolFunctions)
		mcpToolMeta = filterAllowedMCPMeta(mcpToolMeta, opts.AllowedToolFunctions)
	}
	if opts.Cooperative != nil {
		ctx = llmsvc.AppendRequestObserver(ctx, opts.Cooperative.observeRequest)
	}
	usageScope := usagesvc.ScopeFromContext(ctx)
	if usageScope.RootSessionID == "" {
		usageScope.RootSessionID = sessionID
	}
	usageScope.SessionID = sessionID
	if usageScope.WorkingDirectory == "" {
		usageScope.WorkingDirectory = sandboxpolicy.WorkingDirectoryFromContext(ctx)
	}
	ctx = usagesvc.WithScope(ctx, usageScope)
	contextRun := opts.ContextRun
	if contextRun == nil {
		contextRun = contextsvc.Ephemeral(a.providerFactory, contextMessages)
		defer contextRun.Close()
	}
	contextReadAllowed := len(opts.AllowedToolFunctions) == 0
	if _, ok := opts.AllowedToolFunctions["context_read"]; ok {
		contextReadAllowed = true
	}
	if contextReadAllowed {
		toolDefs = append(toolDefs, contextReadToolDef())
	}
	if opts.Cooperative != nil {
		toolDefs = append(toolDefs, cooperativeDefs()...)
	}
	var finalAnswer strings.Builder

	if opts.Depth > 0 {
		opts.steering = nil
	}
	if opts.steering != nil && callbacks.WaitApproval != nil {
		wait := callbacks.WaitApproval
		callbacks.WaitApproval = func(ctx context.Context, id string) (*ApprovalResponse, error) {
			ctx, cancel := opts.steering.approvalContext(ctx)
			defer cancel()
			return wait(ctx, id)
		}
	}
	dispatcher := ToolDispatcher{
		agent: a, model: modelConfig, mcpConfigs: mcpConfigs, mcpMeta: mcpToolMeta,
		contextRun: contextRun, contextReadAllowed: contextReadAllowed, callbacks: callbacks,
		runtime: a.newTurnToolRuntime(sessionID, opts.SandboxPolicy, activatedSkills),
	}

	appendChanges := func(changes []SteeringMessage) error {
		for _, change := range changes {
			if err := contextRun.Append(ctx, llmsvc.ChatMessage{Role: "user", Content: change.Content}); err != nil {
				return err
			}
		}
		return nil
	}
	provider := a.providerFactory.GetProvider(modelConfig.Provider)

	overflowRecoveries := 0
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return finalAnswer.String(), err
		}
		limit := opts.MaxIterations
		if limit <= 0 {
			limit = constants.AgentMaxIterations
		}
		if i >= limit {
			return finalAnswer.String(), fmt.Errorf("agent loop reached max iterations (%d)", limit)
		}
		if opts.Cooperative != nil {
			relays, err := opts.Cooperative.relays(ctx, false)
			if err != nil {
				return finalAnswer.String(), err
			}
			for _, relay := range relays {
				if relay.SourceKind == "human_input" && opts.ApprovalMode == constants.ApprovalModeScheduledAuto {
					opts.ApprovalMode = constants.ApprovalModeStandard
					opts.Cooperative.caller.ApprovalMode = constants.ApprovalModeStandard
				}
			}
			if err := contextRun.Append(ctx, relays...); err != nil {
				return finalAnswer.String(), err
			}
		}

		if err := appendChanges(opts.steering.drain(false)); err != nil {
			return finalAnswer.String(), err
		}
		prepared, prepareErr := contextRun.Prepare(ctx, modelConfig, toolDefs, false)
		if prepareErr != nil {
			return "", prepareErr
		}
		messages := prepared.Messages
		if opts.OnPrepared != nil {
			if err := opts.OnPrepared(prepared); err != nil {
				return "", err
			}
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
		streamCallbacks := llmsvc.StreamCallbacks{
			OnPrivacy: callbacks.OnPrivacy,
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
		}
		requestModel := func() (*llmsvc.StreamResult, error) {
			return provider.StreamChatWithTools(ctx, modelConfig, messages, toolDefs, streamCallbacks)
		}
		result, err := requestModel()
		if err != nil && llmsvc.IsContextWindowExceeded(err) && ctx.Err() == nil && chunkBuf.Len() == 0 && !thinkingStarted && overflowRecoveries < 2 {
			recovered, recoverErr := contextRun.Prepare(ctx, modelConfig, toolDefs, true)
			if recoverErr != nil {
				return "", recoverErr
			}
			if recovered.InputTokens < prepared.InputTokens {
				overflowRecoveries++
				messages = recovered.Messages
				prepared = recovered
				if opts.OnPrepared != nil {
					if err := opts.OnPrepared(recovered); err != nil {
						return "", err
					}
				}
				result, err = requestModel()
			}
		}
		if err != nil {
			return "", fmt.Errorf("agent LLM call failed at iteration %d: %w", i+1, err)
		}
		if result == nil {
			return "", fmt.Errorf("模型返回空响应")
		}

		if result.TokenUsage != nil {
			contextRun.Calibrate(*result.TokenUsage, modelConfig.Provider, contextsvc.Estimate(messages, toolDefs))
		}
		if result.TokenUsage != nil && !result.TokenUsage.IsZero() {
			if opts.LatestUsage != nil {
				*opts.LatestUsage = *result.TokenUsage
			}
			if opts.OnProviderUsage != nil {
				if err := opts.OnProviderUsage(*result.TokenUsage); err != nil {
					return "", fmt.Errorf("OnProviderUsage callback failed: %w", err)
				}
			}
		}

		if thinkingStarted && !thinkingDone {
			if err := finishThinking(); err != nil {
				return "", fmt.Errorf("OnThinkingDone callback failed: %w", err)
			}
		}

		assistant := result.AssistantMessage
		assistant.Role = "assistant"
		if assistant.Content == "" {
			assistant.Content = chunkBuf.String()
		}
		if len(assistant.ToolCalls) == 0 {
			assistant.ToolCalls = result.ToolCalls
		}
		if err := contextRun.Append(ctx, assistant); err != nil {
			return "", fmt.Errorf("记录模型步骤失败: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if result.Type == llmsvc.StreamResultText {
			finalAnswer.WriteString(chunkBuf.String())
			if opts.Cooperative != nil {
				relays, err := opts.Cooperative.relays(ctx, true)
				if err != nil {
					return finalAnswer.String(), err
				}
				if len(relays) > 0 {
					for _, relay := range relays {
						if relay.SourceKind == "human_input" && opts.ApprovalMode == constants.ApprovalModeScheduledAuto {
							opts.ApprovalMode = constants.ApprovalModeStandard
							opts.Cooperative.caller.ApprovalMode = constants.ApprovalModeStandard
						}
					}
					if err := contextRun.Append(ctx, relays...); err != nil {
						return finalAnswer.String(), err
					}
					continue
				}
			}
			if changes := opts.steering.drain(true); len(changes) > 0 {
				if err := appendChanges(changes); err != nil {
					return finalAnswer.String(), err
				}
				continue
			}
			return finalAnswer.String(), nil
		}

		if keepAnswer, err := dispatcher.dispatch(ctx, opts, append(messages, assistant), result.ToolCalls); err != nil {
			if keepAnswer {
				return finalAnswer.String(), err
			}
			return "", err
		}
		// If plan_complete was called, return immediately so the caller can save the plan.
		if opts.PlanComplete != nil && *opts.PlanComplete {
			if changes := opts.steering.drain(true); len(changes) > 0 {
				*opts.PlanComplete = false
				if err := appendChanges(changes); err != nil {
					return finalAnswer.String(), err
				}
				continue
			}
			return finalAnswer.String(), nil
		}
	}
}

// isPlanModeAllowedTool returns true if the tool function name is allowed in plan mode.
func isPlanModeAllowedTool(funcName string) bool {
	return tools.IsPlanModeAllowedFunction(funcName)
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

func filterToolDefsByToolName(defs []llmsvc.ToolDef, toolName string) []llmsvc.ToolDef {
	filtered := make([]llmsvc.ToolDef, 0, len(defs))
	for _, def := range defs {
		name, _, ok := tools.ParseFunctionName(def.Name)
		if ok && name == toolName {
			continue
		}
		if def.Name == toolName {
			continue
		}
		filtered = append(filtered, def)
	}
	return filtered
}

func filterAllowedToolDefs(defs []llmsvc.ToolDef, allowed map[string]struct{}) []llmsvc.ToolDef {
	filtered := make([]llmsvc.ToolDef, 0, len(defs))
	for _, def := range defs {
		if _, ok := allowed[def.Name]; ok {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

func filterAllowedMCPMeta(meta map[string]mcp.ToolMeta, allowed map[string]struct{}) map[string]mcp.ToolMeta {
	filtered := make(map[string]mcp.ToolMeta)
	for name, item := range meta {
		if _, ok := allowed[name]; ok {
			filtered[name] = item
		}
	}
	return filtered
}

func isSilentInternalTool(invocation resolvedToolInvocation) bool {
	return invocation.toolName == constants.MemoryToolName && !invocation.isMCP
}

// filterPlanModeMCPMeta keeps only MCP metadata entries for read-only tools.
func filterPlanModeMCPMeta(meta map[string]mcp.ToolMeta) map[string]mcp.ToolMeta {
	filtered := make(map[string]mcp.ToolMeta)
	for k, v := range meta {
		if isPlanModeAllowedTool(k) {
			filtered[k] = v
		}
	}
	return filtered
}

// parseToolCallName parses "{tool}__{command}" function names.
func parseToolCallName(funcName string) (toolName, command string, err error) {
	toolName, command, ok := tools.ParseFunctionName(funcName)
	if !ok {
		return "", "", fmt.Errorf("invalid tool function name format: %s", funcName)
	}
	return toolName, command, nil
}

// parseToolCallArgs normalizes tool arguments to string maps for built-in tools.
func parseToolCallArgs(arguments string) (map[string]any, error) {
	if strings.TrimSpace(arguments) == "" {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return raw, nil
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

func parseTodoUpdate(arguments string) (TodoUpdate, error) {
	var update TodoUpdate
	if strings.TrimSpace(arguments) == "" {
		return update, fmt.Errorf("arguments are required")
	}
	if err := json.Unmarshal([]byte(arguments), &update); err != nil {
		return update, fmt.Errorf("failed to parse JSON: %w", err)
	}
	if len(update.Items) == 0 {
		return update, fmt.Errorf("items must contain at least one todo")
	}
	inProgress := 0
	allCompleted := true
	seenIDs := make(map[string]struct{}, len(update.Items))
	for i, item := range update.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Content = strings.TrimSpace(item.Content)
		item.Status = strings.TrimSpace(item.Status)
		if item.ID == "" {
			return update, fmt.Errorf("items[%d].id is required", i)
		}
		if _, exists := seenIDs[item.ID]; exists {
			return update, fmt.Errorf("duplicate todo id: %s", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		if item.Content == "" {
			return update, fmt.Errorf("items[%d].content is required", i)
		}
		switch item.Status {
		case "pending":
			allCompleted = false
		case "in_progress":
			inProgress++
			allCompleted = false
		case "completed":
		default:
			return update, fmt.Errorf("items[%d].status must be pending, in_progress, or completed", i)
		}
		update.Items[i] = item
	}
	if !allCompleted && inProgress != 1 {
		return update, fmt.Errorf("todo updates must have exactly one in_progress item unless all items are completed")
	}
	update.Note = strings.TrimSpace(update.Note)
	return update, nil
}

func parseTodoUpdateParams(params map[string]any) (TodoUpdate, error) {
	if params == nil {
		return TodoUpdate{}, fmt.Errorf("arguments are required")
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return TodoUpdate{}, fmt.Errorf("failed to encode params: %w", err)
	}
	return parseTodoUpdate(string(raw))
}

func isSuccessfulBuiltinTodoUpdate(invocation resolvedToolInvocation, result *tools.ExecuteResult) bool {
	if invocation.toolName != "todo" || invocation.command != "update" {
		return false
	}
	if result == nil {
		return false
	}
	return strings.TrimSpace(result.Error) == ""
}

// executeToolCall runs a built-in tool command with uniform error handling.
func executeToolCall(ctx context.Context, toolName, command string, params map[string]any) *tools.ExecuteResult {
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

func determineToolApprovalPolicy(toolName string, isMCP bool, approvalMode string) toolApprovalPolicy {
	if toolName == constants.AskQuestionsTool {
		return toolApprovalPolicyManual
	}
	if approvalMode == constants.ApprovalModeScheduledAuto {
		return toolApprovalPolicyNone
	}
	if isMCP {
		if approvalMode == constants.ApprovalModeAutoReview {
			return toolApprovalPolicyAutoReview
		}
		return toolApprovalPolicyManual
	}
	if approvalMode == constants.ApprovalModeAuto {
		return toolApprovalPolicyNone
	}
	if !tools.IsApprovalSensitiveTool(toolName) {
		return toolApprovalPolicyNone
	}
	if approvalMode == constants.ApprovalModeAutoReview {
		return toolApprovalPolicyAutoReview
	}
	return toolApprovalPolicyManual
}

func (a *AgentService) drainAllProcesses(ctx context.Context) error {
	a.readFilesMu.Lock()
	managers := make([]*tools.ProcessManager, 0, len(a.processesBySess))
	for _, manager := range a.processesBySess {
		managers = append(managers, manager)
	}
	a.readFilesMu.Unlock()
	for _, manager := range managers {
		if err := manager.StopAndWait(ctx); err != nil {
			return err
		}
	}
	return nil
}
