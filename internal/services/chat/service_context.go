package chat

import (
	"context"
	"fmt"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
	prompts "slimebot/prompts"
)

// RunContext 持有部署相关的上下文信息，注入到系统提示词的 Runtime Environment 部分。
// 在启动时构建一次，之后视为不可变。
type RunContext struct {
	// ConfigHomeDir 是 ~/.slimebot 的绝对路径。
	ConfigHomeDir string
	// ConfigDirDescription 是配置目录内容的可读描述，在启动时生成一次。
	ConfigDirDescription string
	// WorkingDir 是 CLI 模式下的当前工作目录，Server 模式为空。
	WorkingDir string
	// IsCLI 为 true 时表示运行在 CLI headless 模式。
	IsCLI bool
}

// BuildContextMessages 构造发给模型的完整上下文消息。
func (s *ChatService) BuildContextMessages(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig) ([]llmsvc.ChatMessage, error) {
	return s.buildContextMessages(ctx, sessionID, modelConfig)
}

// buildContextMessages 并行加载系统提示词和最近历史，再按 system -> memory -> history 顺序组装上下文。
func (s *ChatService) buildContextMessages(ctx context.Context, sessionID string, modelConfig llmsvc.ModelRuntimeConfig) ([]llmsvc.ChatMessage, error) {
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
	logging.Span("context_parallel_system_history", parallelStart)
	if loadErr != nil {
		return nil, loadErr
	}
	if histErr != nil {
		return nil, histErr
	}

	msgs := []llmsvc.ChatMessage{{Role: "system", Content: systemPrompt}}
	if runtimeEnvPrompt := s.buildRuntimeEnvironmentPrompt(); runtimeEnvPrompt != "" {
		msgs = append(msgs, llmsvc.ChatMessage{Role: "system", Content: runtimeEnvPrompt})
	}
	if s.memory != nil {
		memStart := time.Now()
		memCtx, cancel := context.WithTimeout(ctx, constants.MemoryContextBuildBudget)
		memoryContext := s.memory.BuildSessionMemoryContextForPrompt(memCtx, sessionID, history)
		cancel()
		logging.Span("memory_context_build", memStart)
		if memoryContext != "" {
			msgs = append(msgs, llmsvc.ChatMessage{
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
		msgs = append(msgs, llmsvc.ChatMessage{
			Role:    item.Role,
			Content: messageContent,
		})
	}
	logging.Info("chat_context_ready", "session", sessionID, "history", len(history), "mode", "memory_plus_recent20", "cost_ms", time.Since(buildStart).Milliseconds())
	logging.Span("context_build_total", buildStart)
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
