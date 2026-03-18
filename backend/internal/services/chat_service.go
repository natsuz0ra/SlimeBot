package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
)

type ChatService struct {
	repo         *repositories.Repository
	agent        *AgentService
	skillRuntime *SkillRuntimeService
	memory       *MemoryService
	skillsMu     sync.Mutex
	skillsBySess map[string]map[string]struct{}
}

type ChatStreamResult struct {
	Answer         string
	TitleUpdated   bool
	Title          string
	SummaryUpdated bool
	PushFailed     bool
	PushError      string
}

// NewChatService 组装聊天服务及其依赖的 agent/memory 子能力。
func NewChatService(repo *repositories.Repository, openai *OpenAIClient, mcpManager *mcp.Manager, skillRuntime *SkillRuntimeService, memory *MemoryService) *ChatService {
	return &ChatService{
		repo:         repo,
		agent:        NewAgentService(openai, mcpManager, skillRuntime, memory),
		skillRuntime: skillRuntime,
		memory:       memory,
		skillsBySess: make(map[string]map[string]struct{}),
	}
}

func (s *ChatService) getSessionActivatedSkills(sessionID string) map[string]struct{} {
	if strings.TrimSpace(sessionID) == "" {
		return map[string]struct{}{}
	}
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if s.skillsBySess == nil {
		s.skillsBySess = make(map[string]map[string]struct{})
	}
	current := s.skillsBySess[sessionID]
	copyMap := make(map[string]struct{}, len(current))
	for name := range current {
		copyMap[name] = struct{}{}
	}
	return copyMap
}

func (s *ChatService) mergeSessionActivatedSkills(sessionID string, activated map[string]struct{}) {
	if strings.TrimSpace(sessionID) == "" || len(activated) == 0 {
		return
	}
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if s.skillsBySess == nil {
		s.skillsBySess = make(map[string]map[string]struct{})
	}
	existing := s.skillsBySess[sessionID]
	if existing == nil {
		existing = make(map[string]struct{}, len(activated))
		s.skillsBySess[sessionID] = existing
	}
	for name := range activated {
		existing[name] = struct{}{}
	}
}

// EnsureSession 确保会话存在；若不存在则创建默认新会话。
func (s *ChatService) EnsureSession(sessionID string) (*models.Session, error) {
	if sessionID != "" {
		existing, err := s.repo.GetSessionByID(sessionID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}
	return s.repo.CreateSession("New Chat")
}

// EnsureMessagePlatformSession 确保固定的消息平台会话存在。
func (s *ChatService) EnsureMessagePlatformSession() (*models.Session, error) {
	session, err := s.repo.GetSessionByID(consts.MessagePlatformSessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}
	return s.repo.CreateSessionWithID(consts.MessagePlatformSessionID, consts.MessagePlatformSessionName)
}

// ResolvePlatformModel 按平台会话策略解析模型：
// 1) messagePlatformDefaultModel；2) defaultModel；3) 首个可用模型。
// 当平台默认模型不存在时，会自动回退并写回，避免每次请求都失败。
func (s *ChatService) ResolvePlatformModel() (string, error) {
	resolveModel := func(modelID string) (string, bool, error) {
		trimmed := strings.TrimSpace(modelID)
		if trimmed == "" {
			return "", false, nil
		}
		item, err := s.repo.GetLLMConfigByID(trimmed)
		if err != nil {
			return "", false, err
		}
		if item == nil {
			return "", false, nil
		}
		return item.ID, true, nil
	}

	platformDefault, err := s.repo.GetSetting(consts.SettingMessagePlatformDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(platformDefault); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}

	globalDefault, err := s.repo.GetSetting(consts.SettingDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(globalDefault); err != nil {
		return "", err
	} else if ok {
		_ = s.repo.SetSetting(consts.SettingMessagePlatformDefaultModel, id)
		return id, nil
	}

	allModels, err := s.repo.ListLLMConfigs()
	if err != nil {
		return "", err
	}
	if len(allModels) == 0 {
		return "", fmt.Errorf("No available model is configured.")
	}
	fallbackID := strings.TrimSpace(allModels[0].ID)
	if fallbackID == "" {
		return "", fmt.Errorf("No available model is configured.")
	}
	_ = s.repo.SetSetting(consts.SettingMessagePlatformDefaultModel, fallbackID)
	return fallbackID, nil
}

type chatContextBuilder struct {
	service *ChatService
}

// BuildContextMessages 生成模型上下文消息（系统提示 + 历史 + 可选 memory 上下文）。
func (s *ChatService) BuildContextMessages(ctx context.Context, sessionID string, modelConfig ModelRuntimeConfig) ([]ChatMessage, error) {
	builder := chatContextBuilder{service: s}
	return builder.Build(ctx, sessionID, modelConfig)
}

func (b chatContextBuilder) Build(ctx context.Context, sessionID string, modelConfig ModelRuntimeConfig) ([]ChatMessage, error) {
	return b.service.buildContextMessages(ctx, sessionID, modelConfig)
}

func (s *ChatService) buildContextMessages(ctx context.Context, sessionID string, modelConfig ModelRuntimeConfig) ([]ChatMessage, error) {
	_ = ctx
	_ = modelConfig
	buildStart := time.Now()
	systemPrompt, err := s.loadSystemPrompt()
	if err != nil {
		return nil, err
	}

	// 拼接环境信息到系统提示词
	envInfo := CollectEnvInfo()
	systemPrompt = systemPrompt + "\n\n## Runtime Environment\n" + envInfo.FormatForPrompt()
	if s.skillRuntime != nil {
		catalogPrompt, _, catalogErr := s.skillRuntime.BuildCatalogPrompt()
		if catalogErr != nil {
			return nil, catalogErr
		}
		if strings.TrimSpace(catalogPrompt) != "" {
			systemPrompt = systemPrompt + "\n\n" + catalogPrompt
		}
	}

	history, err := s.repo.ListRecentSessionMessages(sessionID, consts.ContextHistoryLimit)
	if err != nil {
		return nil, err
	}

	msgs := []ChatMessage{{Role: "system", Content: systemPrompt}}
	if s.memory != nil {
		sessionSummary := ""
		if memoryItem, memoryErr := s.repo.GetSessionMemory(sessionID); memoryErr != nil {
			log.Printf("chat_context_memory_skip session=%s reason=get_summary_failed err=%v", sessionID, memoryErr)
		} else if memoryItem != nil {
			sessionSummary = strings.TrimSpace(memoryItem.Summary)
		}
		memoryContext := s.memory.FormatCurrentSessionContext(sessionSummary)
		if memoryContext != "" {
			msgs = append(msgs, ChatMessage{
				Role: "system",
				Content: "The following memory_context is provided by the system. Use it primarily to understand historical preferences, constraints, and long-term tasks; " +
					"if it conflicts with the user's current input, always follow the current input.\n\n<memory_context>\n" +
					memoryContext +
					"\n</memory_context>",
			})
		}
	}
	msgs = append(msgs, ChatMessage{
		Role:    "system",
		Content: buildProtocolDeveloperPrompt(time.Now()),
	})

	for _, item := range history {
		msgs = append(msgs, ChatMessage{
			Role:    item.Role,
			Content: item.Content,
		})
	}
	log.Printf("chat_context_ready session=%s history=%d mode=memory_plus_recent20 cost_ms=%d", sessionID, len(history), time.Since(buildStart).Milliseconds())
	return msgs, nil
}

func buildProtocolDeveloperPrompt(now time.Time) string {
	turnTime := now.Local().Format(time.RFC3339)
	return "Final response protocol (strict):\n" +
		"1. Output exactly one <title>...</title> and one <summary>...</summary> block in the final response.\n" +
		"2. Keep title concise and action-oriented, and it must reflect the whole conversation context, not just the latest turn.\n" +
		"3. For every turn, regenerate a NEW summary from scratch by combining: current user request, <memory_context> (if provided), and recent conversation messages.\n" +
		"4. The newly generated summary is the latest canonical memory for this session turn and semantically replaces the previous summary.\n" +
		"5. Never generate title/summary based on the current turn alone.\n" +
		"6. The summary must be detailed, narrative, and can contain multiple paragraphs.\n" +
		"7. The summary must include: this turn's user question time (" + turnTime + "), user intent, and your conclusion.\n" +
		"8. Compress and merge older dialogue details while preserving key turning points and historical continuity.\n" +
		"9. Drop irrelevant branches/options not chosen by the user; when needed, describe as \"multiple options were considered, and the user selected X\".\n" +
		"10. Do not include tool logs, greetings, or markdown headings inside <summary>."
}

// loadSystemPrompt 从 prompts 目录加载系统提示词模板。
func (s *ChatService) loadSystemPrompt() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("Failed to locate the system prompt file path.")
	}

	serviceDir := filepath.Dir(currentFile)
	projectRoot := filepath.Clean(filepath.Join(serviceDir, "..", "..", "prompts"))

	var (
		raw []byte
		err error
	)
	raw, err = os.ReadFile(filepath.Join(projectRoot, "system_prompt.md"))
	if err != nil {
		return "", fmt.Errorf("Failed to read system prompt: %w", err)
	}

	prompt := strings.TrimSpace(string(raw))
	if prompt == "" {
		return "", fmt.Errorf("System prompt is empty.")
	}
	return prompt, nil
}

// ResolveLLMConfig 校验并返回当前会话使用的模型配置。
func (s *ChatService) ResolveLLMConfig(modelID string) (*models.LLMConfig, error) {
	configID := strings.TrimSpace(modelID)
	if configID == "" {
		return nil, fmt.Errorf("modelId is required.")
	}

	config, err := s.repo.GetLLMConfigByID(configID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("Model config not found: %s.", configID)
	}

	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("Model config is incomplete: %s.", config.Name)
	}
	return config, nil
}

// HandleChatStream 使用 Agent 循环处理聊天流。
// 模型可能返回纯文本或 tool_calls，Agent 循环会自动处理工具调用流程。
func (s *ChatService) HandleChatStream(
	ctx context.Context,
	sessionID string,
	requestID string,
	content string,
	modelID string,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("Message cannot be empty.")
	}

	llmConfig, err := s.ResolveLLMConfig(modelID)
	if err != nil {
		return nil, err
	}
	modelConfig := ModelRuntimeConfig{
		BaseURL: llmConfig.BaseURL,
		APIKey:  llmConfig.APIKey,
		Model:   llmConfig.Model,
	}

	session, err := s.repo.GetSessionByID(sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("Session not found: %s.", sessionID)
	}

	if _, err := s.repo.AddMessage(sessionID, "user", content); err != nil {
		return nil, err
	}

	contextMessages, err := s.BuildContextMessages(ctx, sessionID, modelConfig)
	if err != nil {
		return nil, err
	}
	enabledMCPConfigs, err := s.repo.ListEnabledMCPConfigs()
	if err != nil {
		return nil, err
	}

	isTitleLocked := session.IsTitleLocked
	parser := newTitleStreamParser(!isTitleLocked)
	var answerBuilder strings.Builder
	var pushErr error
	streamStart := time.Now()
	var firstTokenAt time.Time

	pushBody := func(body string) error {
		if body == "" {
			return nil
		}
		answerBuilder.WriteString(body)
		if pushErr != nil {
			return nil
		}
		if err := callbacks.OnChunk(body); err != nil {
			pushErr = err
		}
		return nil
	}

	// 包装 OnChunk，经过 title parser
	agentCallbacks := AgentCallbacks{
		OnChunk: func(chunk string) error {
			if chunk != "" && firstTokenAt.IsZero() {
				firstTokenAt = time.Now()
			}
			body := parser.Feed(chunk)
			return pushBody(body)
		},
		OnToolCallStart: func(req ApprovalRequest) error {
			startStatus := consts.ToolCallStatusExecuting
			if req.RequiresApproval {
				startStatus = consts.ToolCallStatusPending
			}
			if err := s.recordToolCallStart(sessionID, requestID, req, startStatus); err != nil {
				return err
			}
			// 进入工具调用阶段前，结束当前回答片段并为下一轮回答重置标题探测状态。
			if err := pushBody(parser.BeginAssistantTurn()); err != nil {
				return err
			}
			if callbacks.OnToolCallStart == nil {
				return nil
			}
			return callbacks.OnToolCallStart(req)
		},
		WaitApproval: callbacks.WaitApproval,
		OnToolCallResult: func(result ToolCallResult) error {
			status := strings.TrimSpace(result.Status)
			if status == "" {
				status = consts.ToolCallStatusCompleted
				if result.Error != "" {
					status = consts.ToolCallStatusError
					if strings.Contains(strings.ToLower(result.Error), "rejected by the user") {
						status = consts.ToolCallStatusRejected
					}
				}
			}
			if err := s.recordToolCallResult(sessionID, requestID, result, status); err != nil {
				return err
			}
			if callbacks.OnToolCallResult == nil {
				return nil
			}
			return callbacks.OnToolCallResult(result)
		},
	}

	activatedSkills := s.getSessionActivatedSkills(sessionID)
	answer, err := s.agent.RunAgentLoop(ctx, modelConfig, sessionID, contextMessages, enabledMCPConfigs, activatedSkills, agentCallbacks)
	s.mergeSessionActivatedSkills(sessionID, activatedSkills)
	if err != nil && answer == "" {
		return nil, err
	}

	firstTokenMs := int64(-1)
	if !firstTokenAt.IsZero() {
		firstTokenMs = firstTokenAt.Sub(streamStart).Milliseconds()
	}
	log.Printf("chat_stream_done session=%s first_token_ms=%d total_stream_ms=%d", sessionID, firstTokenMs, time.Since(streamStart).Milliseconds())

	if err := pushBody(parser.Flush()); err != nil {
		return nil, err
	}

	finalAnswer := answer
	// 兜底解析 <title>/<summary> 协议，避免在多轮 tool_call 场景下元信息丢失。
	// 同时统一净化正文，确保协议标签不会残留存档。
	title := parser.Title()
	summary := parser.Summary()
	if parsedTitle, parsedSummary, cleanBody := extractProtocolMetaAndBody(finalAnswer); parsedTitle != "" || parsedSummary != "" || cleanBody != finalAnswer {
		if parsedTitle != "" {
			title = parsedTitle
		}
		if parsedSummary != "" {
			summary = parsedSummary
		}
		finalAnswer = cleanBody
	}
	if strings.TrimSpace(finalAnswer) == "" {
		finalAnswer = "The model returned no content."
	}
	assistantMessage, err := s.repo.AddMessage(sessionID, "assistant", finalAnswer)
	if err != nil {
		return nil, err
	}
	if err := s.repo.BindToolCallsToAssistantMessage(sessionID, requestID, assistantMessage.ID); err != nil {
		return nil, err
	}
	result := &ChatStreamResult{Answer: finalAnswer}
	if title != "" {
		if err := s.repo.UpdateSessionTitle(sessionID, title); err != nil {
			return nil, err
		}
		result.TitleUpdated = true
		result.Title = title
	}
	if s.memory != nil && strings.TrimSpace(summary) != "" {
		messageCount, countErr := s.repo.CountSessionMessages(sessionID)
		if countErr != nil {
			return nil, countErr
		}
		updated, upsertErr := s.repo.UpsertSessionMemoryIfNewer(repositories.SessionMemoryUpsertInput{
			SessionID:          sessionID,
			Summary:            summary,
			Keywords:           s.memory.TokenizeKeywords(summary),
			SourceMessageCount: int(messageCount),
		})
		if upsertErr != nil {
			return nil, upsertErr
		}
		result.SummaryUpdated = updated
	} else if s.memory != nil {
		log.Printf("chat_summary_missing session=%s reason=empty_or_unparsed", sessionID)
	}
	if pushErr != nil {
		result.PushFailed = true
		result.PushError = pushErr.Error()
	}
	return result, nil
}

func (s *ChatService) recordToolCallStart(
	sessionID string,
	requestID string,
	req ApprovalRequest,
	startStatus string,
) error {
	// 记录工具调用起始状态，后续由结果更新接口补齐结束态。
	return s.repo.UpsertToolCallStart(repositories.ToolCallStartRecordInput{
		SessionID:        sessionID,
		RequestID:        requestID,
		ToolCallID:       req.ToolCallID,
		ToolName:         req.ToolName,
		Command:          req.Command,
		Params:           req.Params,
		Status:           startStatus,
		RequiresApproval: req.RequiresApproval,
		StartedAt:        time.Now(),
	})
}

func (s *ChatService) recordToolCallResult(
	sessionID string,
	requestID string,
	result ToolCallResult,
	status string,
) error {
	// 回写工具调用最终状态，用于会话消息历史回放。
	return s.repo.UpdateToolCallResult(repositories.ToolCallResultRecordInput{
		SessionID:  sessionID,
		RequestID:  requestID,
		ToolCallID: result.ToolCallID,
		Status:     status,
		Output:     result.Output,
		Error:      result.Error,
		FinishedAt: time.Now(),
	})
}
