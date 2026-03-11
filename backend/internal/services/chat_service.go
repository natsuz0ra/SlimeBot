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

	"corner/backend/internal/models"
	"corner/backend/internal/repositories"
)

type ChatService struct {
	repo   *repositories.Repository
	openai *OpenAIClient
	agent  *AgentService
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

func NewChatService(repo *repositories.Repository, openai *OpenAIClient) *ChatService {
	return &ChatService{
		repo:   repo,
		openai: openai,
		agent:  NewAgentService(openai),
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

func (s *ChatService) BuildContextMessages(sessionID string, userInput string) ([]ChatMessage, error) {
	buildStart := time.Now()
	systemPrompt, err := s.loadSystemPrompt()
	if err != nil {
		return nil, err
	}

	// 拼接环境信息到系统提示词
	envInfo := CollectEnvInfo()
	systemPrompt = systemPrompt + "\n\n## 当前运行环境\n" + envInfo.FormatForPrompt()

	history, err := s.repo.ListRecentSessionMessages(sessionID, contextHistoryLimit)
	if err != nil {
		return nil, err
	}
	msgs := []ChatMessage{{Role: "system", Content: systemPrompt}}
	for _, item := range history {
		msgs = append(msgs, ChatMessage{
			Role:    item.Role,
			Content: item.Content,
		})
	}
	msgs = append(msgs, ChatMessage{Role: "user", Content: strings.TrimSpace(userInput)})
	log.Printf("chat_context_ready session=%s history=%d cost_ms=%d", sessionID, len(history), time.Since(buildStart).Milliseconds())
	return msgs, nil
}

func (s *ChatService) loadSystemPrompt() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("无法定位系统提示词文件路径")
	}

	// 优先尝试 .md 文件，兼容 .txt
	promptDir := filepath.Join(filepath.Dir(currentFile), "..", "prompts")
	mdPath := filepath.Join(promptDir, "system_prompt.md")

	var raw []byte
	var err error

	raw, err = os.ReadFile(mdPath)
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

	contextMessages, err := s.BuildContextMessages(sessionID, content)
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
		OnToolCallStart:  callbacks.OnToolCallStart,
		WaitApproval:     callbacks.WaitApproval,
		OnToolCallResult: callbacks.OnToolCallResult,
	}

	modelConfig := ModelRuntimeConfig{
		BaseURL: llmConfig.BaseURL,
		APIKey:  llmConfig.APIKey,
		Model:   llmConfig.Model,
	}

	answer, err := s.agent.RunAgentLoop(ctx, modelConfig, contextMessages, agentCallbacks)
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
	if _, err := s.repo.AddMessage(sessionID, "assistant", finalAnswer); err != nil {
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
	if pushErr != nil {
		result.PushFailed = true
		result.PushError = pushErr.Error()
	}
	return result, nil
}

type titleStreamParser struct {
	enabled bool
	title   string
	lineBuf strings.Builder
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

	writePassthrough := func(content string) {
		if content == "" {
			return
		}
		out.WriteString(content)
	}

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
			p.title = title
			p.lineBuf.Reset()
			p.probing = true
			return
		}
		writePassthrough(current)
		p.lineBuf.Reset()
		p.probing = strings.HasSuffix(current, "\n")
	}

	for i := 0; i < len(chunk); i++ {
		ch := chunk[i]
		if p.probing {
			p.lineBuf.WriteByte(ch)
			trimmedLeft := strings.TrimLeft(p.lineBuf.String(), " \t\r\n\uFEFF")
			titleTag := "[TITLE]"
			if len([]rune(trimmedLeft)) >= len([]rune(titleTag)) && !strings.HasPrefix(trimmedLeft, titleTag) {
				flushLineBuffer(true)
			} else {
				flushLineBuffer(false)
			}
			continue
		}

		out.WriteByte(ch)
		if ch == '\n' {
			p.probing = true
		}
	}

	if flush {
		flushLineBuffer(true)
	}

	return out.String()
}

func (p *titleStreamParser) Title() string {
	return p.title
}

func firstNewlineIndex(input string) (int, int) {
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '\n':
			return i, 1
		case '\r':
			if i+1 < len(input) && input[i+1] == '\n' {
				return i, 2
			}
			return i, 1
		}
	}
	return -1, 0
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
