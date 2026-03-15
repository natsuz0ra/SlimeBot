package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"slimebot/backend/internal/mcp"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
)

type ChatService struct {
	repo         *repositories.Repository
	agent        *AgentService
	skillRuntime *SkillRuntimeService
	memory       *MemoryService
}

const contextHistoryLimit = 20
const titleProbeRuneLimit = 100

type ChatStreamResult struct {
	Answer       string
	TitleUpdated bool
	Title        string
	PushFailed   bool
	PushError    string
}

func NewChatService(repo *repositories.Repository, openai *OpenAIClient, mcpManager *mcp.Manager, skillRuntime *SkillRuntimeService, memory *MemoryService) *ChatService {
	return &ChatService{
		repo:         repo,
		agent:        NewAgentService(openai, mcpManager, skillRuntime),
		skillRuntime: skillRuntime,
		memory:       memory,
	}
}

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
	return s.repo.CreateSession("新会话")
}

func (s *ChatService) BuildContextMessages(ctx context.Context, sessionID string, modelConfig ModelRuntimeConfig) ([]ChatMessage, error) {
	buildStart := time.Now()
	systemPrompt, err := s.loadSystemPrompt()
	if err != nil {
		return nil, err
	}

	// 拼接环境信息到系统提示词
	envInfo := CollectEnvInfo()
	systemPrompt = systemPrompt + "\n\n## 当前运行环境\n" + envInfo.FormatForPrompt()
	if s.skillRuntime != nil {
		catalogPrompt, _, catalogErr := s.skillRuntime.BuildCatalogPrompt()
		if catalogErr != nil {
			return nil, catalogErr
		}
		if strings.TrimSpace(catalogPrompt) != "" {
			systemPrompt = systemPrompt + "\n\n" + catalogPrompt
		}
	}

	history, err := s.repo.ListRecentSessionMessages(sessionID, contextHistoryLimit)
	if err != nil {
		return nil, err
	}

	msgs := []ChatMessage{{Role: "system", Content: systemPrompt}}
	compressEnabled := false
	var memoryKeywords []string
	var memoryHitCount int
	if s.memory != nil {
		shouldCompress, messageCount, countErr := s.memory.ShouldCompressContext(sessionID)
		if countErr != nil {
			log.Printf("chat_context_memory_skip session=%s reason=count_failed err=%v", sessionID, countErr)
		} else if shouldCompress {
			compressEnabled = true

			sessionSummary := ""
			if memoryItem, memoryErr := s.repo.GetSessionMemory(sessionID); memoryErr != nil {
				log.Printf("chat_context_memory_skip session=%s reason=get_summary_failed err=%v", sessionID, memoryErr)
			} else if memoryItem != nil {
				sessionSummary = strings.TrimSpace(memoryItem.Summary)
			}

			lastUserInput := ""
			for idx := len(history) - 1; idx >= 0; idx-- {
				if strings.EqualFold(strings.TrimSpace(history[idx].Role), "user") {
					lastUserInput = strings.TrimSpace(history[idx].Content)
					break
				}
			}

			decision, decideErr := s.memory.DecideMemoryQuery(ctx, modelConfig, lastUserInput, sessionSummary)
			if decideErr != nil {
				log.Printf("chat_context_memory_skip session=%s reason=decision_failed err=%v", sessionID, decideErr)
			} else if decision.NeedMemory {
				memoryKeywords = decision.Keywords
			}

			var memoryHits []repositories.SessionMemorySearchHit
			if len(memoryKeywords) > 0 {
				hits, retrieveErr := s.memory.RetrieveMemories(memoryKeywords, sessionID, memorySearchTopK)
				if retrieveErr != nil {
					log.Printf("chat_context_memory_skip session=%s reason=retrieve_failed err=%v", sessionID, retrieveErr)
				} else {
					memoryHits = hits
					memoryHitCount = len(hits)
				}
			}

			memoryContext := s.memory.FormatMemoryContext(sessionSummary, memoryHits)
			if memoryContext != "" {
				msgs = append(msgs, ChatMessage{
					Role: "developer",
					Content: "以下是系统提供的 memory_context，请优先用于理解历史偏好、约束与长期任务；" +
						"若与用户当前输入冲突，以用户当前输入为准。\n\n<memory_context>\n" +
						memoryContext +
						"\n</memory_context>",
				})
			}

			compactHistory, compactErr := s.memory.BuildCompactHistory(sessionID)
			if compactErr != nil {
				log.Printf("chat_context_memory_skip session=%s reason=compact_history_failed err=%v", sessionID, compactErr)
			} else if len(compactHistory) > 0 {
				history = compactHistory
			}
			log.Printf("chat_context_compressed session=%s message_count=%d keywords=%d hits=%d", sessionID, messageCount, len(memoryKeywords), memoryHitCount)
		}
	}

	for _, item := range history {
		msgs = append(msgs, ChatMessage{
			Role:    item.Role,
			Content: item.Content,
		})
	}
	log.Printf("chat_context_ready session=%s history=%d compressed=%t cost_ms=%d", sessionID, len(history), compressEnabled, time.Since(buildStart).Milliseconds())
	return msgs, nil
}

func (s *ChatService) loadSystemPrompt() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位系统提示词文件路径")
	}

	serviceDir := filepath.Dir(currentFile)
	projectRoot := filepath.Clean(filepath.Join(serviceDir, "..", "..", "prompts"))

	var (
		raw []byte
		err error
	)
	raw, err = os.ReadFile(filepath.Join(projectRoot, "system_prompt.md"))
	if err != nil {
		return "", fmt.Errorf("读取系统提示词失败: %w", err)
	}

	prompt := strings.TrimSpace(string(raw))
	if prompt == "" {
		return "", fmt.Errorf("系统提示词为空")
	}
	return prompt, nil
}

func (s *ChatService) ResolveLLMConfig(modelID string) (*models.LLMConfig, error) {
	configID := strings.TrimSpace(modelID)
	if configID == "" {
		return nil, fmt.Errorf("modelId 不能为空")
	}

	config, err := s.repo.GetLLMConfigByID(configID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("模型配置不存在: %s", configID)
	}

	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("模型配置不完整: %s", config.Name)
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
		return nil, fmt.Errorf("消息不能为空")
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
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
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
	parser := newTitleStreamParser(!isTitleLocked, titleProbeRuneLimit)
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
			startStatus := "executing"
			if req.RequiresApproval {
				startStatus = "pending"
			}
			if err := s.repo.UpsertToolCallStart(repositories.ToolCallStartRecordInput{
				SessionID:        sessionID,
				RequestID:        requestID,
				ToolCallID:       req.ToolCallID,
				ToolName:         req.ToolName,
				Command:          req.Command,
				Params:           req.Params,
				Status:           startStatus,
				RequiresApproval: req.RequiresApproval,
				StartedAt:        time.Now(),
			}); err != nil {
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
				status = "completed"
				if result.Error != "" {
					status = "error"
					if strings.Contains(result.Error, "用户拒绝") {
						status = "rejected"
					}
				}
			}
			if err := s.repo.UpdateToolCallResult(repositories.ToolCallResultRecordInput{
				SessionID:  sessionID,
				RequestID:  requestID,
				ToolCallID: result.ToolCallID,
				Status:     status,
				Output:     result.Output,
				Error:      result.Error,
				FinishedAt: time.Now(),
			}); err != nil {
				return err
			}
			if callbacks.OnToolCallResult == nil {
				return nil
			}
			return callbacks.OnToolCallResult(result)
		},
	}

	activatedSkills := make(map[string]struct{})
	answer, err := s.agent.RunAgentLoop(ctx, modelConfig, contextMessages, enabledMCPConfigs, activatedSkills, agentCallbacks)
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
	// 兜底解析 [TITLE] 协议，避免在多轮 tool_call 场景下标题丢失。
	// 同时统一净化正文，确保不会把 [TITLE] 残留存档。
	title := parser.Title()
	if parsedTitle, cleanBody := extractProtocolTitleAndBody(finalAnswer); parsedTitle != "" {
		title = parsedTitle
		finalAnswer = cleanBody
	}
	if strings.TrimSpace(finalAnswer) == "" {
		finalAnswer = "模型没有返回内容。"
	}
	assistantMessage, err := s.repo.AddMessage(sessionID, "assistant", finalAnswer)
	if err != nil {
		return nil, err
	}
	if err := s.repo.BindToolCallsToAssistantMessage(sessionID, requestID, assistantMessage.ID); err != nil {
		return nil, err
	}
	if s.memory != nil {
		s.memory.UpdateSummaryAsync(modelConfig, sessionID)
	}

	result := &ChatStreamResult{Answer: finalAnswer}
	if title != "" {
		if err := s.repo.UpdateSessionTitle(sessionID, title); err != nil {
			return nil, err
		}
		result.TitleUpdated = true
		result.Title = title
	}
	if pushErr != nil {
		result.PushFailed = true
		result.PushError = pushErr.Error()
	}
	return result, nil
}

type titleStreamParser struct {
	// 是否启用协议解析；关闭时全部透传。
	enabled bool
	// 最近一次成功解析出的标题。
	title string
	// 探测模式下的行缓冲，用于按“行”识别 [TITLE] 协议。
	lineBuf strings.Builder
	// 是否处于探测模式：true=先入缓冲识别协议，false=正文直通。
	probing bool
}

func newTitleStreamParser(enabled bool, probeRuneLimit int) *titleStreamParser {
	_ = probeRuneLimit
	if !enabled {
		return &titleStreamParser{enabled: false}
	}
	return &titleStreamParser{enabled: true, probing: true}
}

func (p *titleStreamParser) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	if !p.enabled {
		return chunk
	}
	return p.process(chunk, false)
}

func (p *titleStreamParser) Flush() string {
	if !p.enabled {
		return ""
	}
	return p.process("", true)
}

func (p *titleStreamParser) process(chunk string, flush bool) string {
	var out strings.Builder

	// 将确认属于正文的内容透传到输出。
	writePassthrough := func(content string) {
		if content == "" {
			return
		}
		out.WriteString(content)
	}

	// 刷新当前行缓冲：可在遇到换行时自然刷新，也可强制刷新（流结束/判定非协议时）。
	flushLineBuffer := func(force bool) {
		current := p.lineBuf.String()
		if current == "" {
			return
		}
		if !force && !strings.Contains(current, "\n") {
			return
		}
		line := strings.TrimSuffix(current, "\n")
		line = strings.TrimSuffix(line, "\r")
		if title, ok := parseProtocolTitle(line); ok {
			// 协议行仅用于更新标题，不进入正文输出。
			p.title = title
			p.lineBuf.Reset()
			// 处理完一行后继续处于探测模式，便于识别下一行协议。
			p.probing = true
			return
		}
		writePassthrough(current)
		p.lineBuf.Reset()
		// 仅当以换行结束时，下一字符才视作新行开头并重新探测。
		p.probing = strings.HasSuffix(current, "\n")
	}

	for i := 0; i < len(chunk); i++ {
		ch := chunk[i]
		if p.probing {
			// 探测模式：先累积到行缓冲，再判断是否为 [TITLE] 协议。
			p.lineBuf.WriteByte(ch)
			trimmedLeft := strings.TrimLeft(p.lineBuf.String(), " \t\r\n\uFEFF")
			titleTag := "[TITLE]"
			// 前缀已足够但不匹配时，立即强制刷新为正文，避免无谓等待整行。
			if len([]rune(trimmedLeft)) >= len([]rune(titleTag)) && !strings.HasPrefix(trimmedLeft, titleTag) {
				flushLineBuffer(true)
			} else {
				flushLineBuffer(false)
			}
			continue
		}

		// 直通模式：正文原样输出，换行后回到探测模式。
		out.WriteByte(ch)
		if ch == '\n' {
			p.probing = true
		}
	}

	if flush {
		// 流结束时强制处理残留缓冲，避免最后半行被遗漏。
		flushLineBuffer(true)
	}

	return out.String()
}

func (p *titleStreamParser) Title() string {
	return p.title
}

func (p *titleStreamParser) BeginAssistantTurn() string {
	if !p.enabled {
		return ""
	}
	// 工具调用切轮时先冲刷残留，再回到“新一行起点探测”状态。
	passthrough := p.process("", true)
	p.probing = true
	return passthrough
}

func parseProtocolTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(line, "\uFEFF", ""))
	if !strings.HasPrefix(trimmed, "[TITLE]") {
		return "", false
	}

	title := strings.TrimSpace(strings.TrimPrefix(trimmed, "[TITLE]"))
	title = strings.ReplaceAll(title, "\r", "")
	title = strings.ReplaceAll(title, "\n", "")
	title = strings.Trim(title, "\"'\u201c\u201d")
	title = truncateRunes(title, 20)
	if title == "" {
		return "", false
	}
	return title, true
}

func extractProtocolTitleAndBody(input string) (string, string) {
	if strings.TrimSpace(input) == "" {
		return "", input
	}

	// 按行扫描协议行，支持 [TITLE] 出现在首行/中间/末行。
	segments := strings.SplitAfter(input, "\n")
	if len(segments) == 0 {
		return "", input
	}

	var extractedTitle string
	var hasTitle bool
	bodySegments := make([]string, 0, len(segments))

	for _, seg := range segments {
		line := strings.TrimSuffix(seg, "\n")
		line = strings.TrimSuffix(line, "\r")
		if title, ok := parseProtocolTitle(line); ok {
			extractedTitle = title
			hasTitle = true
			continue
		}
		bodySegments = append(bodySegments, seg)
	}

	if !hasTitle {
		return "", input
	}

	return extractedTitle, strings.Join(bodySegments, "")
}

func truncateRunes(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max])
}
