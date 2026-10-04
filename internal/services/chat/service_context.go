package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/logging"
	"slimebot/internal/mcp"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	contextsvc "slimebot/internal/services/context"
	llmsvc "slimebot/internal/services/llm"
	"slimebot/internal/tools"
	prompts "slimebot/prompts"
)

// RunContext holds deployment/runtime info for the Runtime Environment section of the system prompt.
// Built once at startup and treated as immutable.
type RunContext struct {
	// ConfigHomeDir is the absolute path to ~/.slimebot.
	ConfigHomeDir string
	// ConfigDirDescription is a human-readable listing of the config dir (computed at startup).
	ConfigDirDescription string
	// WorkingDir is the CLI cwd; empty in server mode.
	WorkingDir string
	// IsCLI is true when running the CLI headless backend.
	IsCLI bool
}

type contextCompressionResult struct {
	messages       []llmsvc.ChatMessage
	compacted      bool
	compactedNow   bool
	compactedAt    string
	meterTools     []llmsvc.ToolDef
	meterConfig    *llmsvc.ModelRuntimeConfig
	lastCheckpoint *domain.ContextCheckpoint
}

type contextBuildResult struct {
	messages     []llmsvc.ChatMessage
	usage        ContextUsage
	compactedNow bool
}

// BuildContextMessages builds the full message list for the model.
func (s *ChatService) BuildContextMessages(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig) ([]llmsvc.ChatMessage, error) {
	result, err := s.buildContextMessagesDetailed(ctx, sessionID, modelConfig)
	if err != nil {
		return nil, err
	}
	return result.messages, nil
}

func (s *ChatService) BuildContextUsage(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig) (ContextUsage, error) {
	result, err := s.buildContextMessagesDetailed(ctx, sessionID, modelConfig)
	if err != nil {
		return ContextUsage{}, err
	}
	return result.usage, nil
}

func (s *ChatService) GetContextUsage(ctx context.Context, sessionID string, modelID string) (ContextUsage, error) {
	usage, _, err := s.GetContextUsageDetailed(ctx, sessionID, modelID)
	return usage, err
}

func (s *ChatService) GetContextUsageDetailed(ctx context.Context, sessionID string, modelID string) (ContextUsage, bool, error) {
	llmConfig, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return ContextUsage{}, false, err
	}
	result, err := s.buildContextMessagesDetailed(ctx, sessionID, llmsvc.ModelRuntimeConfig{
		ConfigID:    llmConfig.ID,
		Provider:    llmConfig.Provider,
		BaseURL:     llmConfig.BaseURL,
		APIKey:      llmConfig.APIKey,
		Model:       llmConfig.Model,
		ContextSize: llmConfig.ContextSize,
	})
	if err != nil {
		return ContextUsage{}, false, err
	}
	return result.usage, result.compactedNow, nil
}

// A negative limit explicitly reads all history; never silently omit the newest input.
const contextCompressionMaxMessages = -1

// buildContextMessages loads context prefix and history in parallel, then orders stable prefix -> dynamic tail -> optional compact summary -> history.
func (s *ChatService) buildContextMessages(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig) ([]llmsvc.ChatMessage, error) {
	result, err := s.buildContextMessagesDetailed(ctx, sessionID, modelConfig)
	if err != nil {
		return nil, err
	}
	return result.messages, nil
}

func (s *ChatService) buildContextMessagesDetailed(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig) (contextBuildResult, error) {
	buildStart := time.Now()
	parallelStart := time.Now()
	var (
		stablePrefix []llmsvc.ChatMessage
		history      []domain.Message
		toolRecords  []domain.ToolCallRecord
		loadErr      error
		histErr      error
		toolErr      error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		prefix, err := s.buildStableContextPrefix(ctx)
		if err != nil {
			loadErr = err
			return
		}
		stablePrefix = prefix
	}()
	go func() {
		defer wg.Done()
		var err error
		history, err = s.store.ListAllSessionMessages(ctx, sessionID, contextCompressionMaxMessages)
		histErr = err
	}()
	wg.Wait()
	logging.Span("context_parallel_system_history", parallelStart)
	if loadErr != nil {
		return contextBuildResult{}, loadErr
	}
	if histErr != nil {
		return contextBuildResult{}, histErr
	}
	assistantIDs := assistantMessageIDs(history)
	if len(assistantIDs) > 0 {
		toolRecords, toolErr = s.store.ListSessionToolCallRecordsByAssistantMessageIDs(ctx, sessionID, assistantIDs)
		if toolErr != nil {
			return contextBuildResult{}, toolErr
		}
	}

	dynamicTail, err := s.buildDynamicContextTail(ctx, sessionID)
	if err != nil {
		return contextBuildResult{}, err
	}
	msgs := make([]llmsvc.ChatMessage, 0, len(stablePrefix)+len(dynamicTail))
	msgs = append(msgs, stablePrefix...)
	msgs = append(msgs, dynamicTail...)

	compression, err := s.applyContextCompression(ctx, sessionID, modelConfig, msgs, history, toolRecords)
	if err != nil {
		return contextBuildResult{}, err
	}
	msgs = append(msgs, compression.messages...)
	mode := "full_history"
	if compression.compacted {
		mode = "compact_summary_plus_recent"
	}
	meterConfig := modelConfig
	meterTools := compression.meterTools
	if compression.meterConfig != nil {
		meterConfig = *compression.meterConfig
	} else if s.agent != nil {
		// Built-in definitions can be read without connecting to MCP or invoking a model.
		meterTools, _, err = s.agent.buildRuntimeToolDefs(ctx, nil, 0)
		if err != nil {
			return contextBuildResult{}, err
		}
		meterTools = append(meterTools, contextReadToolDef())
	}
	usage := buildContextUsage(sessionID, meterConfig, msgs, history, toolRecords, compression.compacted, compression.compactedAt)
	usage.UsedTokens = contextsvc.Estimate(msgs, meterTools)
	if cp := compression.lastCheckpoint; cp != nil {
		usage.CompactionBeforeTokens = cp.BeforeTokens
		usage.CompactionAfterTokens = cp.AfterTokens
		usage.CompactionReason = cp.Reason
	}
	usage = normalizeContextUsagePercentages(usage)
	logging.Info(
		"chat_context_ready",
		"session", sessionID,
		"history_messages", len(history),
		"history_rounds", s.contextHistoryRounds,
		"mode", mode,
		"cost_ms", time.Since(buildStart).Milliseconds(),
	)
	logging.Span("context_build_total", buildStart)
	return contextBuildResult{messages: msgs, usage: usage, compactedNow: compression.compactedNow}, nil
}

func (s *ChatService) buildStableContextPrefix(ctx context.Context) ([]llmsvc.ChatMessage, error) {
	systemPrompt, err := s.loadStableSystemPrompt(ctx)
	if err != nil {
		return nil, err
	}
	return []llmsvc.ChatMessage{{Role: "system", Content: systemPrompt}}, nil
}

func (s *ChatService) buildDynamicContextTail(ctx context.Context, sessionID string) ([]llmsvc.ChatMessage, error) {
	workingDirectory := ""
	if !s.runContext.IsCLI && sessionID != "" {
		session, err := s.store.GetSessionByID(ctx, sessionID)
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return nil, err
		}
		if session != nil {
			workingDirectory = session.WorkingDirectory
		}
	}
	runtimeEnvPrompt := s.buildRuntimeEnvironmentPrompt(workingDirectory)
	projectPrompt := ""
	if workingDirectory != "" && s.agents != nil {
		content, err := s.agents.ReadProject(ctx, workingDirectory)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(content) != "" {
			projectPrompt = "# AGENTS.md instructions for " + workingDirectory + "\n\n<INSTRUCTIONS>\n" + strings.TrimSpace(content) + "\n</INSTRUCTIONS>"
		}
	}
	memoryPrompt := ""
	if s.memory != nil {
		block, err := s.memory.FormatForSystemPrompt(ctx)
		if err != nil {
			return nil, err
		}
		memoryPrompt = strings.TrimSpace(block)
	}
	content := strings.TrimSpace(strings.Join(nonEmptyStrings(runtimeEnvPrompt, projectPrompt, memoryPrompt), "\n\n"))
	if content == "" {
		return nil, nil
	}
	return []llmsvc.ChatMessage{{Role: "system", Content: content}}, nil
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Projection is read-only. Actual compaction happens after all runtime input and tool definitions are known.
func (s *ChatService) applyContextCompression(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig, prefix []llmsvc.ChatMessage, history []domain.Message, toolRecords []domain.ToolCallRecord) (contextCompressionResult, error) {
	fallback := historyToChatMessages(history, toolRecords)
	store, ok := s.store.(contextsvc.Store)
	if !ok {
		return contextCompressionResult{messages: fallback}, nil
	}
	snapshot, err := store.GetContextSnapshot(ctx, sessionID)
	if err != nil {
		return contextCompressionResult{}, err
	}
	if len(snapshot.Entries) == 0 {
		return contextCompressionResult{messages: fallback}, nil
	}
	if s.contexts != nil && !s.contexts.Enabled() {
		snapshot.Checkpoints = nil
	}
	known := map[string]bool{}
	for _, e := range snapshot.Entries {
		if e.SourceMessageID != "" {
			known[e.SourceMessageID] = true
		}
	}
	// New UI messages are merged into a temporary projection; this performs no migration or writes.
	id := int64(-1)
	for _, m := range history {
		if known[m.ID] {
			continue
		}
		for _, cm := range historyToChatMessages([]domain.Message{m}, toolRecordsForHistory([]domain.Message{m}, toolRecords)) {
			b, err := json.Marshal(cm)
			if err != nil {
				return contextCompressionResult{}, err
			}
			kind := cm.Role
			if m.Role == "user" {
				kind = "direct_user"
			} else if cm.Role == "user" {
				kind = "tool_artifact"
			}
			snapshot.Entries = append(snapshot.Entries, domain.ContextEntry{ID: id, MessageSeq: m.Seq, Payload: string(b), SourceKind: kind})
			id--
		}
	}
	messages, compacted, at, err := contextsvc.SnapshotView(snapshot, nil)
	if err != nil {
		return contextCompressionResult{}, err
	}
	result := contextCompressionResult{messages: messages, compacted: compacted, compactedAt: at}
	for _, cp := range snapshot.Checkpoints {
		if result.lastCheckpoint == nil || cp.UpdatedAt.After(result.lastCheckpoint.UpdatedAt) {
			latest := cp
			result.lastCheckpoint = &latest
		}
	}
	if snapshot.Head.LastConfigID == modelConfig.ConfigID && snapshot.Head.LastModel == modelConfig.Model && snapshot.Head.LastWindow == modelConfig.ContextSize && snapshot.Head.LastToolsJSON != "" {
		if json.Unmarshal([]byte(snapshot.Head.LastToolsJSON), &result.meterTools) == nil {
			cfg := modelConfig
			cfg.MaxOutputTokens = snapshot.Head.LastOutputReserve
			result.meterConfig = &cfg
		}
	}
	return result, nil
}

func buildContextUsage(sessionID string, modelConfig llmsvc.ModelRuntimeConfig, messages []llmsvc.ChatMessage, history []domain.Message, toolRecords []domain.ToolCallRecord, compacted bool, compactedAt string) ContextUsage {
	total := modelConfig.ContextSize
	if total <= 0 {
		total = constants.DefaultContextSize
	}
	used := estimateChatMessagesTokens(messages)
	usedPercent := 0
	if total > 0 {
		usedPercent = int(float64(used)*100/float64(total) + 0.5)
	}
	if usedPercent < 0 {
		usedPercent = 0
	}
	if usedPercent > 100 {
		usedPercent = 100
	}
	budget, _ := contextsvc.RequestBudget(modelConfig)
	return ContextUsage{
		InputBudget: budget.HardInput, OutputReserve: budget.Output, Source: "estimated",
		SessionID:        sessionID,
		ModelConfigID:    strings.TrimSpace(modelConfig.ConfigID),
		UsedTokens:       used,
		TotalTokens:      total,
		UsedPercent:      usedPercent,
		AvailablePercent: 100 - usedPercent,
		IsCompacted:      compacted,
		CompactedAt:      compactedAt,
	}
}

func nonZeroTokenUsage(usage llmsvc.TokenUsage) *llmsvc.TokenUsage {
	if usage.IsZero() {
		return nil
	}
	return &usage
}

func historyToChatMessages(history []domain.Message, toolRecords []domain.ToolCallRecord) []llmsvc.ChatMessage {
	recordsByAssistantID := topLevelToolRecordsByAssistantID(toolRecords)
	msgs := make([]llmsvc.ChatMessage, 0, len(history)+len(toolRecords))
	for _, item := range history {
		messageContent := item.Content
		if item.Role == "user" && len(item.Attachments) > 0 {
			messageContent = buildHistoryMessageWithAttachments(item.Content, item.Attachments)
		}
		if item.Role == "assistant" {
			messageContent = StripContentMarkers(messageContent)
			if records := recordsByAssistantID[item.ID]; len(records) > 0 {
				assistantToolMsg, toolMsgs, ok := buildHistoricalToolReplay(records)
				if ok {
					msgs = append(msgs, assistantToolMsg)
					msgs = append(msgs, toolMsgs...)
					if strings.TrimSpace(messageContent) != "" {
						msgs = append(msgs, llmsvc.ChatMessage{Role: item.Role, Content: messageContent})
					}
					continue
				}
			}
		}
		msgs = append(msgs, llmsvc.ChatMessage{Role: item.Role, Content: messageContent})
	}
	return msgs
}

func assistantMessageIDs(history []domain.Message) []string {
	ids := make([]string, 0, len(history))
	for _, item := range history {
		if item.Role == "assistant" && strings.TrimSpace(item.ID) != "" {
			ids = append(ids, strings.TrimSpace(item.ID))
		}
	}
	return ids
}

func toolRecordsForHistory(history []domain.Message, records []domain.ToolCallRecord) []domain.ToolCallRecord {
	if len(history) == 0 || len(records) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(history))
	for _, item := range history {
		if item.Role == "assistant" && strings.TrimSpace(item.ID) != "" {
			ids[strings.TrimSpace(item.ID)] = struct{}{}
		}
	}
	filtered := make([]domain.ToolCallRecord, 0, len(records))
	for _, record := range records {
		if record.AssistantMessageID == nil {
			continue
		}
		if _, ok := ids[strings.TrimSpace(*record.AssistantMessageID)]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func topLevelToolRecordsByAssistantID(records []domain.ToolCallRecord) map[string][]domain.ToolCallRecord {
	byAssistantID := make(map[string][]domain.ToolCallRecord)
	for _, record := range records {
		if record.AssistantMessageID == nil || strings.TrimSpace(*record.AssistantMessageID) == "" {
			continue
		}
		if strings.TrimSpace(record.ParentToolCallID) != "" {
			continue
		}
		if strings.TrimSpace(record.ToolCallID) == "" {
			continue
		}
		key := strings.TrimSpace(*record.AssistantMessageID)
		byAssistantID[key] = append(byAssistantID[key], record)
	}
	return byAssistantID
}

func buildHistoricalToolReplay(records []domain.ToolCallRecord) (llmsvc.ChatMessage, []llmsvc.ChatMessage, bool) {
	calls := make([]llmsvc.ToolCallInfo, 0, len(records))
	toolMsgs := make([]llmsvc.ChatMessage, 0, len(records))
	for _, record := range records {
		funcName := historicalToolFunctionName(record)
		if strings.TrimSpace(funcName) == "" {
			continue
		}
		calls = append(calls, llmsvc.ToolCallInfo{
			ID:        strings.TrimSpace(record.ToolCallID),
			Name:      funcName,
			Arguments: historicalToolArguments(record.ParamsJSON),
		})
		toolMsgs = append(toolMsgs, llmsvc.ChatMessage{
			Role:       "tool",
			ToolCallID: strings.TrimSpace(record.ToolCallID),
			Content:    historicalToolResultContent(record),
		})
	}
	if len(calls) == 0 {
		return llmsvc.ChatMessage{}, nil, false
	}
	return llmsvc.ChatMessage{Role: "assistant", ToolCalls: calls}, toolMsgs, true
}

func historicalToolFunctionName(record domain.ToolCallRecord) string {
	modelFuncName := strings.TrimSpace(record.ModelFuncName)
	if modelFuncName != "" {
		return modelFuncName
	}
	toolName := strings.TrimSpace(record.ToolName)
	command := strings.TrimSpace(record.Command)
	if tools.IsHistoricalStableName(toolName) {
		return toolName
	}
	switch toolName {
	case "":
		return ""
	default:
		if command == "" {
			return ""
		}
		if strings.Contains(toolName, "__") {
			return toolName
		}
		if isLikelyMCPToolRecord(toolName) {
			return mcp.BuildFuncName(toolName, command)
		}
		return tools.ModelFunctionName(toolName, command)
	}
}

func isLikelyMCPToolRecord(toolName string) bool {
	switch toolName {
	case "file_read", "file_edit", "file_write", "web_search", "exec", "http_request", constants.AskQuestionsTool:
		return false
	default:
		return true
	}
}

func historicalToolArguments(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return "{}"
	}
	return trimmed
}

func historicalToolResultContent(record domain.ToolCallRecord) string {
	output := record.Output
	errText := strings.TrimSpace(record.Error)
	if errText == "" && record.Status == constants.ToolCallStatusRejected {
		errText = "Execution was rejected by the user."
	}
	if errText == "" {
		return fmt.Sprintf("Execution result:\n%s", output)
	}
	return fmt.Sprintf("Execution result:\n%s\nError: %s", output, errText)
}

func estimateChatMessagesTokens(msgs []llmsvc.ChatMessage) int { return contextsvc.Estimate(msgs, nil) }
func estimateTextTokens(text string) int                       { return contextsvc.EstimateText(text) }

// loadSystemPrompt reads and caches the embedded system prompt.
func (s *ChatService) loadSystemPrompt() (string, error) {
	if cached := strings.TrimSpace(s.getSystemPromptCached()); cached != "" {
		return cached, nil
	}
	prompt := strings.TrimSpace(prompts.SystemPrompt())
	if prompt == "" {
		return "", fmt.Errorf("embedded system prompt is empty")
	}
	s.setSystemPromptCached(prompt)
	return prompt, nil
}

// loadStableSystemPrompt builds and caches stable system prompt; refreshes when skill catalog or AGENTS.md content changes.
func (s *ChatService) loadStableSystemPrompt(ctx context.Context) (string, error) {
	basePrompt, err := s.loadSystemPrompt()
	if err != nil {
		return "", err
	}

	catalogPrompt := ""
	if s.skillRuntime != nil {
		var catalogErr error
		catalogPrompt, _, catalogErr = s.skillRuntime.BuildCatalogPrompt()
		if catalogErr != nil {
			return "", catalogErr
		}
		catalogPrompt = strings.TrimSpace(catalogPrompt)
	}

	agentsPrompt, err := s.buildAgentsInstructionsPrompt(ctx)
	if err != nil {
		return "", err
	}

	if cachedPrompt, cachedCatalog, cachedAgents := s.getStableSystemPromptCached(); strings.TrimSpace(cachedPrompt) != "" && cachedCatalog == catalogPrompt && cachedAgents == agentsPrompt {
		return cachedPrompt, nil
	}

	stable := basePrompt
	if catalogPrompt != "" {
		stable = stable + "\n\n" + catalogPrompt
	}
	if agentsPrompt != "" {
		stable = stable + "\n\n" + agentsPrompt
	}
	s.setStableSystemPromptCached(stable, catalogPrompt, agentsPrompt)
	return stable, nil
}

func (s *ChatService) buildAgentsInstructionsPrompt(ctx context.Context) (string, error) {
	if s.agents == nil {
		return "", nil
	}

	global, err := s.agents.ReadGlobal(ctx)
	if err != nil {
		return "", err
	}
	globalContent := strings.TrimSpace(global.Content)
	projectContent := ""
	if s.runContext.IsCLI && strings.TrimSpace(s.runContext.WorkingDir) != "" {
		projectContent, err = s.agents.ReadProject(ctx, s.runContext.WorkingDir)
		if err != nil {
			return "", err
		}
		projectContent = strings.TrimSpace(projectContent)
	}

	content := globalContent
	if content != "" && projectContent != "" {
		content += "\n\n--- project-doc ---\n\n" + projectContent
	} else if projectContent != "" {
		content = projectContent
	}
	if content == "" {
		return "", nil
	}

	directory := strings.TrimSpace(s.runContext.WorkingDir)
	if directory == "" {
		directory = strings.TrimSpace(s.runContext.ConfigHomeDir)
	}
	if directory == "" {
		directory = "global"
	}

	var b strings.Builder
	b.WriteString("# AGENTS.md instructions for ")
	b.WriteString(directory)
	b.WriteString("\n\n<INSTRUCTIONS>\n")
	b.WriteString(content)
	b.WriteString("\n</INSTRUCTIONS>")
	return b.String(), nil
}

func (s *ChatService) buildRuntimeEnvironmentPrompt(workingDirectory ...string) string {
	envInfo := CollectEnvInfo()
	body := strings.TrimSpace(envInfo.FormatForPrompt())
	if body == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Runtime Environment\n")
	b.WriteString(body)

	rc := s.runContext
	if rc.ConfigHomeDir != "" {
		b.WriteString("- Config directory: ")
		b.WriteString(rc.ConfigHomeDir)
		b.WriteString("\n")
		if rc.ConfigDirDescription != "" {
			b.WriteString("  Contents:\n")
			for _, line := range strings.Split(rc.ConfigDirDescription, "\n") {
				if strings.TrimSpace(line) != "" {
					b.WriteString("    ")
					b.WriteString(line)
					b.WriteString("\n")
				}
			}
		}
	}

	if rc.IsCLI && rc.WorkingDir != "" {
		b.WriteString("- Current working directory: ")
		b.WriteString(rc.WorkingDir)
		b.WriteString("\n")
	} else if len(workingDirectory) > 0 && workingDirectory[0] != "" {
		b.WriteString("- Current working directory: ")
		b.WriteString(workingDirectory[0])
		b.WriteString("\n")
	}

	return b.String()
}
