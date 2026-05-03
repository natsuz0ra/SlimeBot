package chat

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/logging"
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
	llmConfig, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return ContextUsage{}, err
	}
	return s.BuildContextUsage(ctx, sessionID, llmsvc.ModelRuntimeConfig{
		ConfigID:    llmConfig.ID,
		Provider:    llmConfig.Provider,
		BaseURL:     llmConfig.BaseURL,
		APIKey:      llmConfig.APIKey,
		Model:       llmConfig.Model,
		ContextSize: llmConfig.ContextSize,
	})
}

const contextCompressionMaxMessages = 10000

// buildContextMessages loads system prompt and history in parallel, then orders system -> optional compact summary -> history.
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
		systemPrompt string
		history      []domain.Message
		loadErr      error
		histErr      error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sp, err := s.loadStableSystemPrompt()
		if err != nil {
			loadErr = err
			return
		}
		systemPrompt = sp
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

	msgs := []llmsvc.ChatMessage{{Role: "system", Content: systemPrompt}}
	if runtimeEnvPrompt := s.buildRuntimeEnvironmentPrompt(); runtimeEnvPrompt != "" {
		msgs = append(msgs, llmsvc.ChatMessage{Role: "system", Content: runtimeEnvPrompt})
	}

	compression := s.applyContextCompression(ctx, sessionID, modelConfig, msgs, history)
	msgs = append(msgs, compression.messages...)
	mode := "full_history"
	if compression.compacted {
		mode = "compact_summary_plus_recent"
	}
	usage := buildContextUsage(sessionID, modelConfig, msgs, compression.compacted, compression.compactedAt)
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

func (s *ChatService) applyContextCompression(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig, prefix []llmsvc.ChatMessage, history []domain.Message) contextCompressionResult {
	historyMessages := historyToChatMessages(history)
	if len(historyMessages) == 0 {
		return contextCompressionResult{messages: historyMessages}
	}
	contextSize := modelConfig.ContextSize
	if contextSize <= 0 {
		contextSize = constants.DefaultContextSize
	}
	if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), historyMessages...)) <= contextSize {
		return contextCompressionResult{messages: historyMessages}
	}

	modelConfigID := strings.TrimSpace(modelConfig.ConfigID)
	existing, err := s.store.GetSessionContextSummary(ctx, sessionID, modelConfigID)
	if err == nil && strings.TrimSpace(existing.Summary) != "" {
		kept := messagesAfterSeq(history, existing.SummarizedUntilSeq)
		withExisting := append([]llmsvc.ChatMessage{buildCompactSummaryMessage(existing.Summary)}, historyToChatMessages(kept)...)
		tailCount := s.contextHistoryRounds * 2
		if estimateChatMessagesTokens(append(append([]llmsvc.ChatMessage{}, prefix...), withExisting...)) <= contextSize || len(kept) <= tailCount {
			return contextCompressionResult{messages: withExisting, compacted: true, compactedAt: existing.UpdatedAt.Format(time.RFC3339Nano)}
		}
		split := len(kept) - tailCount
		if split > 0 {
			tail := kept[split:]
			summary, compactErr := s.generateContextSummary(ctx, modelConfig, kept[:split], existing.Summary)
			if compactErr == nil && strings.TrimSpace(summary) != "" {
				lastSeq := kept[split-1].Seq
				compactedAt := time.Now()
				if err := s.store.UpsertSessionContextSummary(ctx, &domain.SessionContextSummary{
					SessionID:               sessionID,
					ModelConfigID:           modelConfigID,
					Summary:                 summary,
					SummarizedUntilSeq:      lastSeq,
					PreCompactTokenEstimate: estimateChatMessagesTokens(historyToChatMessages(kept[:split])),
					UpdatedAt:               compactedAt,
				}); err != nil {
					logging.Warn("context_summary_save_failed", "session", sessionID, "error", err)
				}
				return contextCompressionResult{
					messages:     append([]llmsvc.ChatMessage{buildCompactSummaryMessage(summary)}, historyToChatMessages(tail)...),
					compacted:    true,
					compactedNow: true,
					compactedAt:  compactedAt.Format(time.RFC3339Nano),
				}
			}
			if compactErr != nil {
				logging.Warn("context_summary_generate_failed", "session", sessionID, "error", compactErr)
			}
			return contextCompressionResult{
				messages:    append([]llmsvc.ChatMessage{buildCompactSummaryMessage(existing.Summary)}, historyToChatMessages(tail)...),
				compacted:   true,
				compactedAt: existing.UpdatedAt.Format(time.RFC3339Nano),
			}
		}
		return contextCompressionResult{messages: withExisting, compacted: true, compactedAt: existing.UpdatedAt.Format(time.RFC3339Nano)}
	}
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		logging.Warn("context_summary_load_failed", "session", sessionID, "error", err)
	}

	tailCount := s.contextHistoryRounds * 2
	if tailCount < 2 {
		tailCount = constants.DefaultContextHistoryRounds * 2
	}
	split := len(history) - tailCount
	if split <= 0 {
		return contextCompressionResult{messages: limitTailHistory(historyMessages, tailCount)}
	}
	toSummarize := history[:split]
	tail := history[split:]
	summary, compactErr := s.generateContextSummary(ctx, modelConfig, toSummarize, "")
	if compactErr != nil || strings.TrimSpace(summary) == "" {
		if compactErr != nil {
			logging.Warn("context_summary_generate_failed", "session", sessionID, "error", compactErr)
		}
		return contextCompressionResult{messages: historyToChatMessages(tail)}
	}
	lastSeq := toSummarize[len(toSummarize)-1].Seq
	preCompactEstimate := estimateChatMessagesTokens(historyToChatMessages(toSummarize))
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
		messages:     append([]llmsvc.ChatMessage{buildCompactSummaryMessage(summary)}, historyToChatMessages(tail)...),
		compacted:    true,
		compactedNow: true,
		compactedAt:  compactedAt.Format(time.RFC3339Nano),
	}
}

func buildContextUsage(sessionID string, modelConfig llmsvc.ModelRuntimeConfig, messages []llmsvc.ChatMessage, compacted bool, compactedAt string) ContextUsage {
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

func (s *ChatService) generateContextSummary(ctx context.Context, modelConfig llmsvc.ModelRuntimeConfig, history []domain.Message, priorSummary string) (string, error) {
	if s.providerFactory == nil {
		return "", fmt.Errorf("provider factory is not initialized")
	}
	var transcript strings.Builder
	if strings.TrimSpace(priorSummary) != "" {
		transcript.WriteString("已有摘要：\n")
		transcript.WriteString(strings.TrimSpace(priorSummary))
		transcript.WriteString("\n\n")
	}
	for _, item := range historyToChatMessages(history) {
		transcript.WriteString(strings.ToUpper(item.Role))
		transcript.WriteString(": ")
		transcript.WriteString(strings.TrimSpace(item.Content))
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

func historyToChatMessages(history []domain.Message) []llmsvc.ChatMessage {
	msgs := make([]llmsvc.ChatMessage, 0, len(history))
	for _, item := range history {
		messageContent := item.Content
		if item.Role == "user" && len(item.Attachments) > 0 {
			messageContent = buildHistoryMessageWithAttachments(item.Content, item.Attachments)
		}
		if item.Role == "assistant" {
			messageContent = StripContentMarkers(messageContent)
		}
		msgs = append(msgs, llmsvc.ChatMessage{Role: item.Role, Content: messageContent})
	}
	return msgs
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

func limitTailHistory(msgs []llmsvc.ChatMessage, limit int) []llmsvc.ChatMessage {
	if limit <= 0 || len(msgs) <= limit {
		return msgs
	}
	return msgs[len(msgs)-limit:]
}

func estimateChatMessagesTokens(msgs []llmsvc.ChatMessage) int {
	total := 0
	for _, msg := range msgs {
		total += 4
		total += estimateTextTokens(msg.Role)
		total += estimateTextTokens(msg.Content)
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
