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
	llmsvc "slimebot/internal/services/llm"
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
	messages     []llmsvc.ChatMessage
	compacted    bool
	compactedNow bool
	compactedAt  string
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

const contextCompressionMaxMessages = 10000

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
		prefix, err := s.buildStableContextPrefix()
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

	dynamicTail := s.buildDynamicContextTail()
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
	usage := buildContextUsage(sessionID, modelConfig, msgs, history, toolRecords, compression.compacted, compression.compactedAt)
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

func (s *ChatService) buildStableContextPrefix() ([]llmsvc.ChatMessage, error) {
	systemPrompt, err := s.loadStableSystemPrompt()
	if err != nil {
		return nil, err
	}
	return []llmsvc.ChatMessage{{Role: "system", Content: systemPrompt}}, nil
}

func (s *ChatService) buildDynamicContextTail() []llmsvc.ChatMessage {
	runtimeEnvPrompt := s.buildRuntimeEnvironmentPrompt()
	if runtimeEnvPrompt == "" {
		return nil
	}
	return []llmsvc.ChatMessage{{Role: "system", Content: runtimeEnvPrompt}}
}

func (s *ChatService) applyContextCompression(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig, prefix []llmsvc.ChatMessage, history []domain.Message, toolRecords []domain.ToolCallRecord) (contextCompressionResult, error) {
	historyMessages := historyToChatMessages(history, toolRecords)
	if len(historyMessages) == 0 {
		return contextCompressionResult{messages: historyMessages}, nil
	}
	contextSize := modelConfig.ContextSize
	if contextSize <= 0 {
		contextSize = constants.DefaultContextSize
	}
	preserveLatestUser := history[len(history)-1].Role == "user"
	if preserveLatestUser {
		latest := historyToChatMessages(history[len(history)-1:], toolRecordsForHistory(history[len(history)-1:], toolRecords))
		if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), latest...)) > contextSize {
			return contextCompressionResult{}, fmt.Errorf("最新输入超过模型上下文窗口，请缩短输入或调大上下文大小。")
		}
	}

	modelConfigID := strings.TrimSpace(modelConfig.ConfigID)
	existing, err := s.store.GetSessionContextSummary(ctx, sessionID, modelConfigID)
	if err == nil && strings.TrimSpace(existing.Summary) != "" {
		kept := messagesAfterSeq(history, existing.SummarizedUntilSeq)
		keptToolRecords := toolRecordsForHistory(kept, toolRecords)
		existingSummary := []llmsvc.ChatMessage{buildCompactSummaryMessage(existing.Summary)}
		withExisting := append(append([]llmsvc.ChatMessage{}, existingSummary...), historyToChatMessages(kept, keptToolRecords)...)
		if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), withExisting...)) <= contextSize {
			return contextCompressionResult{messages: withExisting, compacted: true, compactedAt: existing.UpdatedAt.Format(time.RFC3339Nano)}, nil
		}

		if len(kept) == 0 {
			return contextCompressionResult{}, fmt.Errorf("压缩摘要仍超过模型上下文窗口，请调大 context size 或新建会话。")
		}
		summary, compactErr := s.generateContextSummary(ctx, modelConfig, kept, keptToolRecords, existing.Summary)
		if compactErr != nil {
			logging.Warn("context_summary_generate_failed", "session", sessionID, "error", compactErr)
			return contextCompressionResult{}, fmt.Errorf("上下文压缩失败: %w", compactErr)
		}
		if strings.TrimSpace(summary) == "" {
			return contextCompressionResult{}, fmt.Errorf("上下文压缩失败: 压缩摘要为空")
		}
		compactedMessages := []llmsvc.ChatMessage{buildCompactSummaryMessage(summary)}
		if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), compactedMessages...)) > contextSize {
			return contextCompressionResult{}, fmt.Errorf("压缩摘要仍超过模型上下文窗口，请调大 context size 或新建会话。")
		}
		lastSeq := kept[len(kept)-1].Seq
		compactedAt := time.Now()
		if err := s.store.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{
			SessionID:               sessionID,
			ModelConfigID:           modelConfigID,
			Summary:                 summary,
			SummarizedUntilSeq:      lastSeq,
			PreCompactTokenEstimate: estimateChatMessagesTokens(historyToChatMessages(kept, keptToolRecords)),
			UpdatedAt:               compactedAt,
		}); err != nil {
			logging.Warn("context_summary_save_failed", "session", sessionID, "error", err)
		}
		return contextCompressionResult{
			messages:     compactedMessages,
			compacted:    true,
			compactedNow: true,
			compactedAt:  compactedAt.Format(time.RFC3339Nano),
		}, nil
	}
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		logging.Warn("context_summary_load_failed", "session", sessionID, "error", err)
	}

	if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), historyMessages...)) <= contextSize {
		return contextCompressionResult{messages: historyMessages}, nil
	}

	summary, compactErr := s.generateContextSummary(ctx, modelConfig, history, toolRecords, "")
	if compactErr != nil || strings.TrimSpace(summary) == "" {
		if compactErr != nil {
			logging.Warn("context_summary_generate_failed", "session", sessionID, "error", compactErr)
			return contextCompressionResult{}, fmt.Errorf("上下文压缩失败: %w", compactErr)
		}
		return contextCompressionResult{}, fmt.Errorf("上下文压缩失败: 压缩摘要为空")
	}
	compactedMessages := []llmsvc.ChatMessage{buildCompactSummaryMessage(summary)}
	if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), compactedMessages...)) > contextSize {
		return contextCompressionResult{}, fmt.Errorf("压缩摘要仍超过模型上下文窗口，请调大 context size 或新建会话。")
	}
	lastSeq := history[len(history)-1].Seq
	preCompactEstimate := estimateChatMessagesTokens(historyMessages)
	compactedAt := time.Now()
	if err := s.store.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{
		SessionID:               sessionID,
		ModelConfigID:           modelConfigID,
		Summary:                 summary,
		SummarizedUntilSeq:      lastSeq,
		PreCompactTokenEstimate: preCompactEstimate,
		UpdatedAt:               compactedAt,
	}); err != nil {
		logging.Warn("context_summary_save_failed", "session", sessionID, "error", err)
	}
	return contextCompressionResult{
		messages:     compactedMessages,
		compacted:    true,
		compactedNow: true,
		compactedAt:  compactedAt.Format(time.RFC3339Nano),
	}, nil
}

func buildContextUsage(sessionID string, modelConfig llmsvc.ModelRuntimeConfig, messages []llmsvc.ChatMessage, history []domain.Message, toolRecords []domain.ToolCallRecord, compacted bool, compactedAt string) ContextUsage {
	total := modelConfig.ContextSize
	if total <= 0 {
		total = constants.DefaultContextSize
	}
	used := estimateChatMessagesTokens(messages)
	if !compacted {
		if exactUsed, ok := contextUsageFromPersistedTokenUsage(history, toolRecords); ok {
			used = exactUsed
		}
	}
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
	return ContextUsage{
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

func contextUsageFromPersistedTokenUsage(history []domain.Message, toolRecords []domain.ToolCallRecord) (int, bool) {
	for i := len(history) - 1; i >= 0; i-- {
		item := history[i]
		if item.Role != "assistant" || item.TokenUsage == nil || item.TokenUsage.IsZero() {
			continue
		}
		used := item.TokenUsage.ContextWindowTokens()
		if i+1 < len(history) {
			tail := history[i+1:]
			used += estimateChatMessagesTokens(historyToChatMessages(tail, toolRecordsForHistory(tail, toolRecords)))
		}
		return used, true
	}
	return 0, false
}

func nonZeroTokenUsage(usage llmsvc.TokenUsage) *llmsvc.TokenUsage {
	if usage.IsZero() {
		return nil
	}
	return &usage
}

func (s *ChatService) generateContextSummary(ctx context.Context, modelConfig llmsvc.ModelRuntimeConfig, history []domain.Message, toolRecords []domain.ToolCallRecord, priorSummary string) (string, error) {
	if s.providerFactory == nil {
		return "", fmt.Errorf("provider factory is not initialized")
	}
	var transcript strings.Builder
	if strings.TrimSpace(priorSummary) != "" {
		transcript.WriteString("已有摘要：\n")
		transcript.WriteString(strings.TrimSpace(priorSummary))
		transcript.WriteString("\n\n")
	}
	for _, item := range historyToChatMessages(history, toolRecords) {
		transcript.WriteString(strings.ToUpper(item.Role))
		transcript.WriteString(": ")
		transcript.WriteString(strings.TrimSpace(formatChatMessageForSummary(item)))
		transcript.WriteString("\n\n")
	}
	prompt := "请对以下对话生成压缩总结，用于后续继续上下文。压缩总结必须保留用户意图、关键决策、涉及的文件/代码、错误与修复、待办和下一步；不要调用工具，只输出摘要正文。\n\n压缩总结输入：\n" + transcript.String()
	provider := s.providerFactory.GetProvider(modelConfig.Provider)
	var summary strings.Builder
	_, err := provider.StreamChatWithTools(ctx, modelConfig, []llmsvc.ChatMessage{
		{Role: "system", Content: "你是会话上下文压缩器。只输出压缩总结正文，不要使用工具。"},
		{Role: "user", Content: prompt},
	}, nil, llmsvc.StreamCallbacks{OnChunk: func(chunk string) error {
		summary.WriteString(chunk)
		return nil
	}})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(summary.String()), nil
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
	toolName := strings.TrimSpace(record.ToolName)
	command := strings.TrimSpace(record.Command)
	switch toolName {
	case "":
		return ""
	case constants.ActivateSkillTool, constants.RunSubagentTool, constants.TodoUpdateTool, constants.PlanStartTool, constants.PlanCompleteTool:
		return toolName
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
		return toolName + "__" + command
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

func formatChatMessageForSummary(msg llmsvc.ChatMessage) string {
	var parts []string
	if strings.TrimSpace(msg.Content) != "" {
		parts = append(parts, strings.TrimSpace(msg.Content))
	}
	for _, tc := range msg.ToolCalls {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			name = "unknown_tool"
		}
		args := historicalToolArguments(tc.Arguments)
		parts = append(parts, fmt.Sprintf("Tool call %s: %s", name, args))
	}
	if msg.Role == "tool" && strings.TrimSpace(msg.ToolCallID) != "" && len(parts) > 0 {
		parts[0] = fmt.Sprintf("Tool result for %s:\n%s", strings.TrimSpace(msg.ToolCallID), parts[0])
	}
	return strings.Join(parts, "\n")
}

func buildCompactSummaryMessage(summary string) llmsvc.ChatMessage {
	return llmsvc.ChatMessage{
		Role: "system",
		Content: "The earlier conversation has been compacted by the system. Use this summary as hidden continuity context, " +
			"and follow newer user messages if they conflict.\n\n<context_summary>\n" +
			strings.TrimSpace(summary) +
			"\n</context_summary>",
	}
}

func messagesAfterSeq(history []domain.Message, seq int64) []domain.Message {
	var kept []domain.Message
	for _, item := range history {
		if item.Seq > seq {
			kept = append(kept, item)
		}
	}
	return kept
}

func estimateChatMessagesTokens(msgs []llmsvc.ChatMessage) int {
	total := 0
	for _, msg := range msgs {
		total += 4
		total += estimateTextTokens(msg.Role)
		total += estimateTextTokens(msg.Content)
		total += estimateTextTokens(msg.ToolCallID)
		total += estimateTextTokens(msg.ReasoningContent)
		for _, tc := range msg.ToolCalls {
			total += estimateTextTokens(tc.ID)
			total += estimateTextTokens(tc.Name)
			total += estimateTextTokens(tc.Arguments)
		}
		for _, block := range msg.ThinkingBlocks {
			total += estimateTextTokens(block.Thinking)
			total += estimateTextTokens(block.Signature)
			total += estimateTextTokens(block.RedactedData)
		}
		for _, part := range msg.ContentParts {
			total += estimateTextTokens(part.Text)
			total += estimateTextTokens(part.ImageURL)
			total += estimateTextTokens(part.Filename)
		}
	}
	return total
}

func estimateTextTokens(text string) int {
	runes := len([]rune(text))
	if runes == 0 {
		return 0
	}
	return (runes + 3) / 4
}

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

// loadStableSystemPrompt builds and caches stable system prompt; refreshes when skill catalog changes.
func (s *ChatService) loadStableSystemPrompt() (string, error) {
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

	if cachedPrompt, cachedCatalog := s.getStableSystemPromptCached(); strings.TrimSpace(cachedPrompt) != "" && cachedCatalog == catalogPrompt {
		return cachedPrompt, nil
	}

	stable := basePrompt
	if catalogPrompt != "" {
		stable = stable + "\n\n" + catalogPrompt
	}
	s.setStableSystemPromptCached(stable, catalogPrompt)
	return stable, nil
}

func (s *ChatService) buildRuntimeEnvironmentPrompt() string {
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
	}

	return b.String()
}
