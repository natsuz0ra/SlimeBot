package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/observability"
	oaisvc "slimebot/internal/services/openai"
	prompts "slimebot/prompts"
)

// BuildContextMessages 构造发给模型的完整上下文消息。
func (s *ChatService) BuildContextMessages(ctx context.Context, sessionID string, modelConfig oaisvc.ModelRuntimeConfig) ([]oaisvc.ChatMessage, error) {
	return s.buildContextMessages(ctx, sessionID, modelConfig)
}

// buildContextMessages 并行加载系统提示词和最近历史，再按 system -> memory -> history 顺序组装上下文。
func (s *ChatService) buildContextMessages(ctx context.Context, sessionID string, modelConfig oaisvc.ModelRuntimeConfig) ([]oaisvc.ChatMessage, error) {
	_ = modelConfig
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
		history, err = s.store.ListRecentSessionMessages(ctx, sessionID, constants.ContextHistoryLimit)
		histErr = err
	}()
	wg.Wait()
	observability.Span("context_parallel_system_history", parallelStart)
	if loadErr != nil {
		return nil, loadErr
	}
	if histErr != nil {
		return nil, histErr
	}

	msgs := []oaisvc.ChatMessage{{Role: "system", Content: systemPrompt}}
	if runtimeEnvPrompt := s.buildRuntimeEnvironmentPrompt(); runtimeEnvPrompt != "" {
		msgs = append(msgs, oaisvc.ChatMessage{Role: "system", Content: runtimeEnvPrompt})
	}
	if s.memory != nil {
		memStart := time.Now()
		memCtx, cancel := context.WithTimeout(ctx, constants.MemoryContextBuildBudget)
		memoryContext := s.memory.BuildSessionMemoryContextForPrompt(memCtx, sessionID, history)
		cancel()
		observability.Span("memory_context_build", memStart)
		if memoryContext != "" {
			msgs = append(msgs, oaisvc.ChatMessage{
				Role: "system",
				Content: "The following memory_context is provided by the system. Use it primarily to understand historical preferences, constraints, and long-term tasks; " +
					"if it conflicts with the user's current input, always follow the current input.\n\n<memory_context>\n" +
					memoryContext +
					"\n</memory_context>",
			})
		}
	}

	for _, item := range history {
		messageContent := item.Content
		if item.Role == "user" && len(item.Attachments) > 0 {
			messageContent = buildHistoryMessageWithAttachments(item.Content, item.Attachments)
		}
		msgs = append(msgs, oaisvc.ChatMessage{
			Role:    item.Role,
			Content: messageContent,
		})
	}
	slog.Info("chat_context_ready", "session", sessionID, "history", len(history), "mode", "memory_plus_recent20", "cost_ms", time.Since(buildStart).Milliseconds())
	observability.Span("context_build_total", buildStart)
	return msgs, nil
}

// loadSystemPrompt 读取并缓存内嵌 system prompt。
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

// loadStableSystemPrompt 构建并缓存稳定前缀 system prompt，仅在技能目录变化时刷新。
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
	return "## Runtime Environment\n" + body
}
