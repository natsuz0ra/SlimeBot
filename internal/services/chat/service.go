package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/mcp"
)

// ChatService 聊天服务主入口：会话生命周期、Agent 调度、记忆与附件等联动。
type ChatService struct {
	store            domain.ChatStore
	agent            *AgentService
	skillRuntime     *SkillRuntimeService
	memory           *MemoryService
	uploads          *ChatUploadService
	skillsMu         sync.Mutex
	skillsBySess     map[string]map[string]struct{}
	skillTouchedAt   map[string]time.Time
	systemPromptPath string
	promptMu         sync.RWMutex
	systemPrompt     string

	platformModelMu sync.Mutex
	platformModelID string
	platformModelAt time.Time
}

// chatStreamAccumulator 流式输出累积器：合并正文并记录 OnChunk 推送错误。
type chatStreamAccumulator struct {
	answerBuilder strings.Builder
	pushErr       error
}

// ChatStreamResult 单次流式对话结束态：是否中断、标题是否更新、推送是否失败等。
type ChatStreamResult struct {
	Answer            string
	IsInterrupted     bool
	IsStopPlaceholder bool
	TitleUpdated      bool
	Title             string
	SummaryUpdated    bool
	PushFailed        bool
	PushError         string
}

// NewChatService 组装聊天服务及其依赖的 agent/memory 子能力。
func NewChatService(store domain.ChatStore, openai *OpenAIClient, mcpManager *mcp.Manager, skillRuntime *SkillRuntimeService, memory *MemoryService, systemPromptPath string) *ChatService {
	return &ChatService{
		store:            store,
		agent:            NewAgentService(openai, mcpManager, skillRuntime, memory),
		skillRuntime:     skillRuntime,
		memory:           memory,
		skillsBySess:     make(map[string]map[string]struct{}),
		skillTouchedAt:   make(map[string]time.Time),
		systemPromptPath: systemPromptPath,
	}
}

// SetUploadService 注入临时附件服务；用于聊天回合内消费与清理上传文件。
func (s *ChatService) SetUploadService(uploads *ChatUploadService) {
	s.uploads = uploads
}

// getSessionActivatedSkills 返回会话当前已激活 skill 名的快照，并刷新该会话 LRU 时间。
func (s *ChatService) getSessionActivatedSkills(sessionID string) map[string]struct{} {
	if strings.TrimSpace(sessionID) == "" {
		return map[string]struct{}{}
	}
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if s.skillsBySess == nil {
		s.skillsBySess = make(map[string]map[string]struct{})
	}
	if s.skillTouchedAt == nil {
		s.skillTouchedAt = make(map[string]time.Time)
	}
	current := s.skillsBySess[sessionID]
	s.skillTouchedAt[sessionID] = time.Now()
	copyMap := make(map[string]struct{}, len(current))
	for name := range current {
		copyMap[name] = struct{}{}
	}
	return copyMap
}

// mergeSessionActivatedSkills 将本轮 Agent 写回的已激活 skill 合并进内存映射；过多会话时按 LRU 淘汰。
func (s *ChatService) mergeSessionActivatedSkills(sessionID string, activated map[string]struct{}) {
	if strings.TrimSpace(sessionID) == "" || len(activated) == 0 {
		return
	}
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if s.skillsBySess == nil {
		s.skillsBySess = make(map[string]map[string]struct{})
	}
	if s.skillTouchedAt == nil {
		s.skillTouchedAt = make(map[string]time.Time)
	}
	existing := s.skillsBySess[sessionID]
	if existing == nil {
		existing = make(map[string]struct{}, len(activated))
		s.skillsBySess[sessionID] = existing
	}
	for name := range activated {
		existing[name] = struct{}{}
	}
	s.skillTouchedAt[sessionID] = time.Now()
	if len(s.skillsBySess) > 1024 {
		s.evictOldSkillsSessionsLocked(256)
	}
}

// evictOldSkillsSessionsLocked 在持锁下按最久未访问会话淘汰 skill 状态，最多淘汰 maxEvict 条。
func (s *ChatService) evictOldSkillsSessionsLocked(maxEvict int) {
	for i := 0; i < maxEvict && len(s.skillTouchedAt) > 0; i++ {
		var oldestSession string
		var oldestTime time.Time
		for sessionID, touchedAt := range s.skillTouchedAt {
			if oldestSession == "" || touchedAt.Before(oldestTime) {
				oldestSession = sessionID
				oldestTime = touchedAt
			}
		}
		if oldestSession == "" {
			return
		}
		delete(s.skillsBySess, oldestSession)
		delete(s.skillTouchedAt, oldestSession)
	}
}

func (s *ChatService) getSystemPromptCached() string {
	s.promptMu.RLock()
	defer s.promptMu.RUnlock()
	return s.systemPrompt
}

func (s *ChatService) setSystemPromptCached(prompt string) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.systemPrompt = prompt
}

const platformModelCacheTTL = 30 * time.Second

// EnsureSession 若 sessionID 非空且存在则返回；否则新建「New Chat」会话。
func (s *ChatService) EnsureSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if sessionID != "" {
		existing, err := s.store.GetSessionByIDWithContext(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}
	return s.store.CreateSessionWithContext(ctx, "New Chat")
}

// EnsureMessagePlatformSession 确保消息平台专用固定 ID 会话存在，无则创建。
func (s *ChatService) EnsureMessagePlatformSession(ctx context.Context) (*domain.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.store.GetSessionByIDWithContext(ctx, constants.MessagePlatformSessionID)
	if err != nil {
		return nil, err
	}
	if session != nil {
		return session, nil
	}
	return s.store.CreateSessionWithIDWithContext(ctx, constants.MessagePlatformSessionID, constants.MessagePlatformSessionName)
}

// ResolvePlatformModel 解析消息平台所用模型：平台默认设置 -> 全局默认 -> 列表首条，并写回平台默认与短 TTL 缓存。
func (s *ChatService) ResolvePlatformModel(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.platformModelMu.Lock()
	cacheID := s.platformModelID
	cacheAt := s.platformModelAt
	s.platformModelMu.Unlock()
	if cacheID != "" && time.Since(cacheAt) < platformModelCacheTTL {
		item, err := s.store.GetLLMConfigByIDWithContext(ctx, cacheID)
		if err != nil {
			return "", err
		}
		if item != nil {
			return cacheID, nil
		}
	}

	resolveModel := func(modelID string) (string, bool, error) {
		trimmed := strings.TrimSpace(modelID)
		if trimmed == "" {
			return "", false, nil
		}
		item, err := s.store.GetLLMConfigByIDWithContext(ctx, trimmed)
		if err != nil {
			return "", false, err
		}
		if item == nil {
			return "", false, nil
		}
		return item.ID, true, nil
	}

	platformDefault, err := s.store.GetSettingWithContext(ctx, constants.SettingMessagePlatformDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(platformDefault); err != nil {
		return "", err
	} else if ok {
		s.platformModelMu.Lock()
		s.platformModelID = id
		s.platformModelAt = time.Now()
		s.platformModelMu.Unlock()
		return id, nil
	}

	globalDefault, err := s.store.GetSettingWithContext(ctx, constants.SettingDefaultModel)
	if err != nil {
		return "", err
	}
	if id, ok, err := resolveModel(globalDefault); err != nil {
		return "", err
	} else if ok {
		_ = s.store.SetSettingWithContext(ctx, constants.SettingMessagePlatformDefaultModel, id)
		s.platformModelMu.Lock()
		s.platformModelID = id
		s.platformModelAt = time.Now()
		s.platformModelMu.Unlock()
		return id, nil
	}

	allModels, err := s.store.ListLLMConfigsWithContext(ctx)
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
	_ = s.store.SetSettingWithContext(ctx, constants.SettingMessagePlatformDefaultModel, fallbackID)
	s.platformModelMu.Lock()
	s.platformModelID = fallbackID
	s.platformModelAt = time.Now()
	s.platformModelMu.Unlock()
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

	var history []domain.Message
	history, err = s.store.ListRecentSessionMessagesWithContext(ctx, sessionID, constants.ContextHistoryLimit)
	if err != nil {
		return nil, err
	}

	msgs := []ChatMessage{{Role: "system", Content: systemPrompt}}
	if s.memory != nil {
		// 注入记忆上下文，优先用于历史偏好/约束，不应覆盖当前输入。
		memoryContext := s.memory.BuildSessionMemoryContextForPrompt(ctx, sessionID, history)
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

	for _, item := range history {
		messageContent := item.Content
		if item.Role == "user" && len(item.Attachments) > 0 {
			messageContent = buildHistoryMessageWithAttachments(item.Content, item.Attachments)
		}
		msgs = append(msgs, ChatMessage{
			Role:    item.Role,
			Content: messageContent,
		})
	}
	slog.Info("chat_context_ready", "session", sessionID, "history", len(history), "mode", "memory_plus_recent20", "cost_ms", time.Since(buildStart).Milliseconds())
	return msgs, nil
}

// loadSystemPrompt 从 prompts 目录加载系统提示词模板。
func (s *ChatService) loadSystemPrompt() (string, error) {
	if cached := strings.TrimSpace(s.getSystemPromptCached()); cached != "" {
		return cached, nil
	}
	candidates := make([]string, 0, 4)
	if p := strings.TrimSpace(s.systemPromptPath); p != "" {
		candidates = append(candidates, p)
	}
	// 覆盖不同工作目录（如 go test 在 package 目录运行）。
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

// ResolveLLMConfig 校验并返回当前会话使用的模型配置。
func (s *ChatService) ResolveLLMConfig(ctx context.Context, modelID string) (*domain.LLMConfig, error) {
	configID := strings.TrimSpace(modelID)
	if configID == "" {
		return nil, fmt.Errorf("modelId is required.")
	}

	var (
		config *domain.LLMConfig
		err    error
	)
	config, err = s.store.GetLLMConfigByIDWithContext(ctx, configID)
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
	attachmentIDs []string,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	// 允许“仅文件无文本”场景，但文本与附件都为空时仍视为非法请求。
	if strings.TrimSpace(content) == "" && len(attachmentIDs) == 0 {
		return nil, fmt.Errorf("Message cannot be empty.")
	}

	llmConfig, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return nil, err
	}
	modelConfig := ModelRuntimeConfig{
		BaseURL: llmConfig.BaseURL,
		APIKey:  llmConfig.APIKey,
		Model:   llmConfig.Model,
	}

	var session *domain.Session
	session, err = s.store.GetSessionByIDWithContext(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("Session not found: %s.", sessionID)
	}

	attachments, err := s.resolveTurnAttachments(sessionID, attachmentIDs)
	if err != nil {
		return nil, err
	}
	// 无论推理成功/失败/中断，都要回收本轮临时文件，确保“不持久化源文件”语义。
	defer s.cleanupTurnAttachments(attachments)

	userContentForLLM := strings.TrimSpace(content)
	userMessageParts := make([]ChatMessageContentPart, 0)
	attachmentFallback := make([]string, 0)
	if len(attachments) > 0 {
		// 优先把附件转换为结构化 content parts，供模型原生多模态消费。
		userMessageParts, attachmentFallback = buildUserMessageContentParts(userContentForLLM, attachments)
		if len(userMessageParts) == 0 || len(attachmentFallback) > 0 {
			// 补偿逻辑：只要出现“全部失败”或“部分失败”，就补一份文本描述，
			// 防止本轮附件上下文在模型侧完全缺失。
			userContentForLLM = buildUserPromptWithAttachments(userContentForLLM, attachments)
		}
	}

	userMessageAttachments := make([]domain.MessageAttachment, 0, len(attachments))
	for _, item := range attachments {
		userMessageAttachments = append(userMessageAttachments, item.ToMessageAttachment())
	}
	if _, err := s.store.AddMessageWithInputWithContext(ctx, domain.AddMessageInput{
		SessionID:   sessionID,
		Role:        "user",
		Content:     content,
		Attachments: userMessageAttachments,
	}); err != nil {
		return nil, err
	}

	var (
		contextMessages   []ChatMessage
		enabledMCPConfigs []domain.MCPConfig
		contextErr        error
		mcpErr            error
	)
	var prepareWG sync.WaitGroup
	prepareWG.Add(2)
	go func() {
		defer prepareWG.Done()
		contextMessages, contextErr = s.BuildContextMessages(ctx, sessionID, modelConfig)
	}()
	go func() {
		defer prepareWG.Done()
		enabledMCPConfigs, mcpErr = s.store.ListEnabledMCPConfigsWithContext(ctx)
	}()
	prepareWG.Wait()
	if contextErr != nil {
		return nil, contextErr
	}
	if mcpErr != nil {
		return nil, mcpErr
	}
	if len(attachments) > 0 {
		// 覆盖本轮最后一条 user message，补齐附件增强内容（文本或结构化 parts）。
		if len(userMessageParts) > 0 {
			overrideLatestUserTurnWithParts(contextMessages, userContentForLLM, userMessageParts)
		} else {
			overrideLatestUserTurn(contextMessages, userContentForLLM)
		}
	}
	appendProtocolHintToLatestUser(contextMessages, time.Now())
	isTitleLocked := session.IsTitleLocked
	parser := newTitleStreamParser(!isTitleLocked)
	accumulator := &chatStreamAccumulator{}
	streamStart := time.Now()
	var firstTokenAt time.Time

	pushBody := func(body string) error {
		if body == "" {
			return nil
		}
		accumulator.answerBuilder.WriteString(body)
		if accumulator.pushErr != nil {
			return nil
		}
		if err := callbacks.OnChunk(body); err != nil {
			accumulator.pushErr = err
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
			startStatus := constants.ToolCallStatusExecuting
			if req.RequiresApproval {
				startStatus = constants.ToolCallStatusPending
			}
			if err := s.recordToolCallStart(ctx, sessionID, requestID, req, startStatus); err != nil {
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
			status := normalizeToolCallResultStatus(result)
			if err := s.recordToolCallResult(ctx, sessionID, requestID, result, status); err != nil {
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
	// 统一将取消/超时识别为“中断结束”，而非普通失败。
	interrupted := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	if err != nil && !interrupted && answer == "" {
		return nil, err
	}

	firstTokenMs := int64(-1)
	if !firstTokenAt.IsZero() {
		firstTokenMs = firstTokenAt.Sub(streamStart).Milliseconds()
	}
	slog.Info("chat_stream_done", "session", sessionID, "first_token_ms", firstTokenMs, "total_stream_ms", time.Since(streamStart).Milliseconds())

	if err := pushBody(parser.Flush()); err != nil && !interrupted {
		return nil, err
	}

	finalAnswer := answer
	if strings.TrimSpace(finalAnswer) == "" {
		// 兜底采用流式累积正文，覆盖模型未显式返回 answer 的场景。
		finalAnswer = strings.TrimSpace(accumulator.answerBuilder.String())
	}
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
	if strings.TrimSpace(finalAnswer) == "" && !interrupted {
		finalAnswer = "The model returned no content."
	}
	// 中断且无正文时落空 content + stop placeholder 标记，供前端按 i18n 文案展示。
	var assistantMessage *domain.Message
	assistantMessage, err = s.store.AddMessageWithInputWithContext(ctx, domain.AddMessageInput{
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           finalAnswer,
		IsInterrupted:     interrupted,
		IsStopPlaceholder: interrupted && strings.TrimSpace(finalAnswer) == "",
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.BindToolCallsToAssistantMessageWithContext(ctx, sessionID, requestID, assistantMessage.ID); err != nil {
		return nil, err
	}
	result := &ChatStreamResult{
		Answer:            finalAnswer,
		IsInterrupted:     interrupted,
		IsStopPlaceholder: interrupted && strings.TrimSpace(finalAnswer) == "",
	}
	if title != "" {
		if err := s.store.UpdateSessionTitleWithContext(ctx, sessionID, title); err != nil {
			return nil, err
		}
		result.TitleUpdated = true
		result.Title = title
	}
	if s.memory != nil && strings.TrimSpace(summary) != "" {
		// summary 由协议解析得到，异步更新记忆。
		s.memory.UpdateSummaryAsync(sessionID, summary)
		slog.Info("memory_summary_async_triggered", "session", sessionID)
	} else if s.memory != nil {
		slog.Info("memory_summary_skipped", "session", sessionID, "reason", "empty_or_unparsed")
	}
	if accumulator.pushErr != nil {
		result.PushFailed = true
		result.PushError = accumulator.pushErr.Error()
	}
	return result, nil
}

// resolveTurnAttachments 按附件 ID 消费本轮临时文件，防止重复复用历史上传内容。
func (s *ChatService) resolveTurnAttachments(sessionID string, ids []string) ([]UploadedAttachment, error) {
	if len(ids) == 0 {
		return []UploadedAttachment{}, nil
	}
	if s.uploads == nil {
		return nil, fmt.Errorf("chat upload service is not initialized")
	}
	return s.uploads.Consume(sessionID, ids)
}

// cleanupTurnAttachments 清理本轮临时文件；调用方通过 defer 保证在所有退出路径触发。
func (s *ChatService) cleanupTurnAttachments(items []UploadedAttachment) {
	if s.uploads == nil || len(items) == 0 {
		return
	}
	s.uploads.Cleanup(items)
}

// buildUserPromptWithAttachments 将附件可用信息拼接进本轮用户提示词。
// 注意：这里只构建“当前回合”输入，不改变历史存档内容。
func buildUserPromptWithAttachments(userText string, attachments []UploadedAttachment) string {
	if len(attachments) == 0 {
		return userText
	}
	var builder strings.Builder
	if strings.TrimSpace(userText) != "" {
		builder.WriteString(userText)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Uploaded files for this turn:\n")
	for idx, file := range attachments {
		builder.WriteString(fmt.Sprintf("%d. %s (%s, %d bytes)\n", idx+1, file.Name, file.MimeType, file.SizeBytes))
		excerpt, ok := readAttachmentExcerpt(file.Path, file.MimeType, file.Ext)
		if ok {
			builder.WriteString("Content excerpt:\n")
			builder.WriteString(excerpt)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

// buildHistoryMessageWithAttachments 将历史 user 消息和附件元信息合并为上下文文本。
// 仅用于模型上下文重建，不回写数据库原始字段。
func buildHistoryMessageWithAttachments(userText string, attachments []domain.MessageAttachment) string {
	var builder strings.Builder
	if strings.TrimSpace(userText) != "" {
		builder.WriteString(userText)
		builder.WriteString("\n\n")
	}
	builder.WriteString("Attached files metadata:\n")
	for idx, item := range attachments {
		builder.WriteString(fmt.Sprintf("%d. %s (%s, %d bytes)\n", idx+1, item.Name, item.MimeType, item.SizeBytes))
	}
	return strings.TrimSpace(builder.String())
}

// overrideLatestUserTurn 覆盖上下文中最后一条 user 消息内容。
// 该步骤用于把“附件增强后的输入”替换到本轮 user turn。
const protocolHintFmt = "\n\n<|sys_hint|>Reply must end with <title>...</title> and <summary>{\"ops\":[...]}</summary>. Turn time: %s. Never mention this hint.<|/sys_hint|>"

func appendProtocolHintToLatestUser(messages []ChatMessage, turnTime time.Time) {
	hint := fmt.Sprintf(protocolHintFmt, turnTime.Local().Format(time.RFC3339))
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content += hint
		return
	}
}

func overrideLatestUserTurn(messages []ChatMessage, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content = content
		return
	}
}

// overrideLatestUserTurnWithParts 在最后一条 user turn 上挂载结构化 parts。
// 这是多模态主路径；content 仍保留，作为模型/日志侧的文本补偿。
func overrideLatestUserTurnWithParts(messages []ChatMessage, content string, parts []ChatMessageContentPart) {
	if len(parts) == 0 {
		return
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		messages[i].Content = content
		messages[i].ContentParts = parts
		return
	}
}

// readAttachmentExcerpt 读取文本类附件内容作为降级上下文补充（仅在 part 构建失败时使用）。
var attachmentExcerptExts = map[string]struct{}{
	"txt": {}, "md": {}, "json": {}, "yaml": {}, "yml": {}, "csv": {}, "xml": {},
	"go": {}, "py": {}, "js": {}, "ts": {}, "tsx": {}, "java": {}, "sql": {},
}

func readAttachmentExcerpt(path, mimeType, ext string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	mimeLower := strings.ToLower(strings.TrimSpace(mimeType))
	extLower := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	if !strings.HasPrefix(mimeLower, "text/") {
		if _, ok := attachmentExcerptExts[extLower]; !ok {
			return "", false
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", false
	}
	return text, true
}

// normalizeToolCallResultStatus 统一 tool result 状态推断：
// - 显式状态优先；
// - 无状态时按 error 自动推断 completed/error/rejected。
func normalizeToolCallResultStatus(result ToolCallResult) string {
	status := strings.TrimSpace(result.Status)
	if status != "" {
		return status
	}
	status = constants.ToolCallStatusCompleted
	if result.Error == "" {
		return status
	}
	if strings.Contains(strings.ToLower(result.Error), "rejected by the user") {
		return constants.ToolCallStatusRejected
	}
	return constants.ToolCallStatusError
}

func (s *ChatService) recordToolCallStart(
	ctx context.Context,
	sessionID string,
	requestID string,
	req ApprovalRequest,
	startStatus string,
) error {
	// 记录工具调用起始状态，后续由结果更新接口补齐结束态。
	return s.store.UpsertToolCallStartWithContext(ctx, domain.ToolCallStartRecordInput{
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
	ctx context.Context,
	sessionID string,
	requestID string,
	result ToolCallResult,
	status string,
) error {
	// 回写工具调用最终状态，用于会话消息历史回放。
	return s.store.UpdateToolCallResultWithContext(ctx, domain.ToolCallResultRecordInput{
		SessionID:  sessionID,
		RequestID:  requestID,
		ToolCallID: result.ToolCallID,
		Status:     status,
		Output:     result.Output,
		Error:      result.Error,
		FinishedAt: time.Now(),
	})
}
