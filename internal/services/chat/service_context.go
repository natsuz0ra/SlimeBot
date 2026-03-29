package chat

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/observability"
	oaisvc "slimebot/internal/services/openai"
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
		sp, err := s.loadSystemPrompt()
		if err != nil {
			loadErr = err
			return
		}
		envInfo := CollectEnvInfo()
		sp = sp + "\n\n## Runtime Environment\n" + envInfo.FormatForPrompt()
		if s.skillRuntime != nil {
			catalogPrompt, _, catalogErr := s.skillRuntime.BuildCatalogPrompt()
			if catalogErr != nil {
				loadErr = catalogErr
				return
			}
			if strings.TrimSpace(catalogPrompt) != "" {
				sp = sp + "\n\n" + catalogPrompt
			}
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

// loadSystemPrompt 按候选路径读取并缓存 system prompt，减少每轮聊天重复读盘。
func (s *ChatService) loadSystemPrompt() (string, error) {
	if cached := strings.TrimSpace(s.getSystemPromptCached()); cached != "" {
		return cached, nil
	}
	candidates := make([]string, 0, 4)
	if p := strings.TrimSpace(s.systemPromptPath); p != "" {
		candidates = append(candidates, p)
	}
	candidates = append(candidates,
		"./prompts/system_prompt.md",
		"../prompts/system_prompt.md",
		"../../prompts/system_prompt.md",
	)

	var lastErr error
	for _, p := range candidates {
		raw, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		prompt := strings.TrimSpace(string(raw))
		if prompt == "" {
			continue
		}
		s.setSystemPromptCached(prompt)
		return prompt, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("system prompt file not found")
	}
	return "", fmt.Errorf("Failed to read system prompt: %w", lastErr)
}
