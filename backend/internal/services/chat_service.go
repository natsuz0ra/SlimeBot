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

	promptPath := filepath.Join(filepath.Dir(currentFile), "..", "prompts", "system_prompt.txt")
	raw, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("读取系统提示词失败: %w", err)
	}

	prompt := strings.TrimSpace(string(raw))
	if prompt == "" {
		return "", fmt.Errorf("系统提示词为空: %s", promptPath)
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

func (s *ChatService) HandleChatStream(
	ctx context.Context,
	sessionID string,
	content string,
	modelID string,
	onChunk func(chunk string) error,
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
		if err := onChunk(body); err != nil {
			pushErr = err
		}
		return nil
	}

	err = s.openai.StreamChat(ctx, ModelRuntimeConfig{
		BaseURL: llmConfig.BaseURL,
		APIKey:  llmConfig.APIKey,
		Model:   llmConfig.Model,
	}, contextMessages, func(chunk string) error {
		if chunk != "" && firstTokenAt.IsZero() {
			firstTokenAt = time.Now()
		}
		body := parser.Feed(chunk)
		return pushBody(body)
	})
	if err != nil {
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

	answer := answerBuilder.String()
	if strings.TrimSpace(answer) == "" {
		answer = "模型没有返回内容。"
	}
	if _, err := s.repo.AddMessage(sessionID, "assistant", answer); err != nil {
		return nil, err
	}

	result := &ChatStreamResult{Answer: answer}
	if parser.Title() != "" {
		if err := s.repo.UpdateSessionTitle(sessionID, parser.Title()); err != nil {
			return nil, err
		}
		result.TitleUpdated = true
		result.Title = parser.Title()
	}
	if pushErr != nil {
		result.PushFailed = true
		result.PushError = pushErr.Error()
	}
	return result, nil
}

type titleStreamParser struct {
	enabled        bool
	resolved       bool
	title          string
	probeRuneLimit int
	buffer         strings.Builder
}

func newTitleStreamParser(enabled bool, probeRuneLimit int) *titleStreamParser {
	if !enabled {
		return &titleStreamParser{enabled: false, resolved: true}
	}
	if probeRuneLimit <= 0 {
		probeRuneLimit = 80
	}
	return &titleStreamParser{enabled: true, probeRuneLimit: probeRuneLimit}
}

func (p *titleStreamParser) Feed(chunk string) string {
	if chunk == "" {
		return ""
	}
	if !p.enabled || p.resolved {
		return chunk
	}

	p.buffer.WriteString(chunk)
	current := p.buffer.String()
	trimmedLeft := strings.TrimLeft(current, " \t\r\n\uFEFF")
	titleTag := "[TITLE]"
	if len([]rune(trimmedLeft)) >= len([]rune(titleTag)) && !strings.HasPrefix(trimmedLeft, titleTag) {
		p.resolved = true
		p.buffer.Reset()
		return current
	}
	newlineIndex, newlineLen := firstNewlineIndex(current)
	if newlineIndex < 0 {
		if len([]rune(current)) > p.probeRuneLimit {
			p.resolved = true
			p.buffer.Reset()
			return current
		}
		return ""
	}

	firstLine := current[:newlineIndex]
	rest := current[newlineIndex+newlineLen:]
	p.resolved = true
	p.buffer.Reset()

	if title, ok := parseProtocolTitle(firstLine); ok {
		p.title = title
		return rest
	}
	return firstLine + current[newlineIndex:newlineIndex+newlineLen] + rest
}

func (p *titleStreamParser) Flush() string {
	if !p.enabled || p.resolved {
		return ""
	}

	current := p.buffer.String()
	p.buffer.Reset()
	p.resolved = true

	if title, ok := parseProtocolTitle(current); ok {
		p.title = title
		return ""
	}
	return current
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
	title = strings.Trim(title, "\"'“”")
	title = truncateRunes(title, 20)
	if title == "" {
		return "", false
	}
	return title, true
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
