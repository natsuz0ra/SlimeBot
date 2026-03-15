package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-ego/gse"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
)

const (
	compressHistoryThreshold       = 10
	compactRawHistoryLimit         = 6
	memorySearchTopK               = 5
	memoryDecisionTimeout          = 12 * time.Second
	memorySummaryTimeout           = 25 * time.Second
	memorySummaryRecentMessageSize = 30
	memoryKeywordMaxCount          = 12
)

var defaultMemoryStopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "for": {}, "to": {}, "of": {}, "in": {}, "on": {}, "at": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "with": {}, "from": {}, "this": {}, "that": {}, "it": {},
	"你": {}, "我": {}, "他": {}, "她": {}, "它": {}, "我们": {}, "你们": {}, "他们": {}, "以及": {}, "并且": {}, "或者": {},
	"一个": {}, "一些": {}, "可以": {}, "需要": {}, "然后": {}, "就是": {}, "这里": {}, "这个": {}, "那个": {},
}

var nonWordRuneRegex = regexp.MustCompile(`[^\p{L}\p{N}_\-]+`)

type MemoryDecision struct {
	NeedMemory bool     `json:"need_memory"`
	Keywords   []string `json:"keywords"`
	Reason     string   `json:"reason"`
}

type MemoryService struct {
	repo      *repositories.Repository
	openai    *OpenAIClient
	segOnce   sync.Once
	segmenter gse.Segmenter
}

func NewMemoryService(repo *repositories.Repository, openai *OpenAIClient) *MemoryService {
	return &MemoryService{
		repo:   repo,
		openai: openai,
	}
}

func (m *MemoryService) ShouldCompressContext(sessionID string) (bool, int64, error) {
	total, err := m.repo.CountSessionMessages(sessionID)
	if err != nil {
		return false, 0, err
	}
	return total >= compressHistoryThreshold, total, nil
}

func (m *MemoryService) DecideMemoryQuery(ctx context.Context, modelConfig ModelRuntimeConfig, userInput string, summary string) (MemoryDecision, error) {
	ctx, cancel := context.WithTimeout(ctx, memoryDecisionTimeout)
	defer cancel()

	systemPrompt := `你是“记忆检索决策器”。请根据用户当前输入和会话摘要，判断是否需要检索历史记忆来回答问题。
仅返回 JSON，不要输出任何额外文本。
JSON 格式：
{"need_memory":true/false,"keywords":["关键词1","关键词2"],"reason":"简短原因"}
要求：
1. 只有在用户问题依赖历史事实、偏好、长期任务或跨会话信息时，need_memory=true。
2. keywords 仅保留 1~8 个可检索关键词，避免停用词。
3. 若不需要检索，keywords 返回空数组。`

	userPrompt := fmt.Sprintf("用户输入：\n%s\n\n当前会话摘要：\n%s", strings.TrimSpace(userInput), strings.TrimSpace(summary))
	reply, err := m.chatOnce(ctx, modelConfig, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return MemoryDecision{}, err
	}

	decision, err := parseMemoryDecision(reply)
	if err != nil {
		return MemoryDecision{}, err
	}
	decision.Keywords = m.TokenizeKeywords(strings.Join(decision.Keywords, " "))
	if !decision.NeedMemory {
		decision.Keywords = nil
	}
	return decision, nil
}

func (m *MemoryService) RetrieveMemories(keywords []string, excludeSessionID string, limit int) ([]repositories.SessionMemorySearchHit, error) {
	if limit <= 0 {
		limit = memorySearchTopK
	}
	return m.repo.SearchMemoriesByKeywords(m.TokenizeKeywords(strings.Join(keywords, " ")), limit, excludeSessionID)
}

func (m *MemoryService) FormatMemoryContext(summary string, hits []repositories.SessionMemorySearchHit) string {
	var b strings.Builder
	if strings.TrimSpace(summary) != "" {
		b.WriteString("<current_session_summary>\n")
		b.WriteString(strings.TrimSpace(summary))
		b.WriteString("\n</current_session_summary>\n")
	}
	if len(hits) > 0 {
		b.WriteString("\n<retrieved_memories>\n")
		for idx, hit := range hits {
			b.WriteString(fmt.Sprintf("  <memory index=\"%d\" session_id=\"%s\" matched=\"%s\">\n", idx+1, hit.Memory.SessionID, strings.Join(hit.MatchedKeywords, ",")))
			b.WriteString("    ")
			b.WriteString(strings.TrimSpace(hit.Memory.Summary))
			b.WriteString("\n  </memory>\n")
		}
		b.WriteString("</retrieved_memories>")
	}
	return strings.TrimSpace(b.String())
}

func (m *MemoryService) BuildCompactHistory(sessionID string) ([]models.Message, error) {
	return m.repo.ListRecentSessionMessages(sessionID, compactRawHistoryLimit)
}

func (m *MemoryService) UpdateSummaryAsync(modelConfig ModelRuntimeConfig, sessionID string) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("memory_summary_panic session=%s recovered=%v", sessionID, recovered)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), memorySummaryTimeout)
		defer cancel()

		totalMessages, err := m.repo.CountSessionMessages(sessionID)
		if err != nil {
			log.Printf("memory_summary_skip session=%s reason=count_failed err=%v", sessionID, err)
			return
		}
		if totalMessages == 0 {
			return
		}

		recent, err := m.repo.ListRecentSessionMessages(sessionID, memorySummaryRecentMessageSize)
		if err != nil {
			log.Printf("memory_summary_skip session=%s reason=recent_failed err=%v", sessionID, err)
			return
		}
		if len(recent) == 0 {
			return
		}

		existing, err := m.repo.GetSessionMemory(sessionID)
		if err != nil {
			log.Printf("memory_summary_skip session=%s reason=get_existing_failed err=%v", sessionID, err)
			return
		}

		oldSummary := ""
		if existing != nil {
			oldSummary = existing.Summary
		}

		mergedSummary, err := m.MergeSummary(ctx, modelConfig, oldSummary, recent)
		if err != nil {
			log.Printf("memory_summary_skip session=%s reason=merge_failed err=%v", sessionID, err)
			return
		}
		keywords := m.TokenizeKeywords(mergedSummary + "\n" + flattenMessages(recent))
		if err := m.repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
			SessionID:          sessionID,
			Summary:            mergedSummary,
			Keywords:           keywords,
			SourceMessageCount: int(totalMessages),
		}); err != nil {
			log.Printf("memory_summary_skip session=%s reason=upsert_failed err=%v", sessionID, err)
			return
		}

		log.Printf("memory_summary_updated session=%s total_messages=%d keywords=%d", sessionID, totalMessages, len(keywords))
	}()
}

func (m *MemoryService) MergeSummary(ctx context.Context, modelConfig ModelRuntimeConfig, oldSummary string, recent []models.Message) (string, error) {
	systemPrompt := `你是会话摘要器。请将“历史摘要”和“最新对话片段”融合成新的高质量记忆摘要。
输出要求：
1. 只输出摘要正文，不要使用 markdown 标题，不要输出 JSON。
2. 保留：用户偏好、关键事实、已完成/待完成任务、重要约束、上下文线索。
3. 删除：寒暄、重复信息、无关工具日志。
4. 摘要尽量精炼，但不要丢失关键信息。`
	userPrompt := fmt.Sprintf("历史摘要：\n%s\n\n最新对话片段：\n%s", strings.TrimSpace(oldSummary), flattenMessages(recent))

	reply, err := m.chatOnce(ctx, modelConfig, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	if err != nil {
		return "", err
	}

	summary := strings.TrimSpace(reply)
	if summary == "" {
		return "", fmt.Errorf("摘要生成为空")
	}
	return summary, nil
}

func (m *MemoryService) chatOnce(ctx context.Context, modelConfig ModelRuntimeConfig, messages []ChatMessage) (string, error) {
	var builder strings.Builder
	err := m.openai.StreamChat(ctx, modelConfig, messages, func(chunk string) error {
		builder.WriteString(chunk)
		return nil
	})
	if err != nil {
		return "", err
	}
	return builder.String(), nil
}

func (m *MemoryService) TokenizeKeywords(text string) []string {
	m.segOnce.Do(func() {
		// 使用嵌入词典快速初始化，避免每次分词重复加载。
		m.segmenter.LoadDict()
	})

	candidates := m.segmenter.CutSearch(strings.TrimSpace(text), true)
	if len(candidates) == 0 {
		candidates = splitByUnicodeWord(text)
	}

	result := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, token := range candidates {
		normalized := normalizeToken(token)
		if normalized == "" {
			continue
		}
		if _, skip := defaultMemoryStopWords[normalized]; skip {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
		if len(result) >= memoryKeywordMaxCount {
			break
		}
	}
	return result
}

func parseMemoryDecision(raw string) (MemoryDecision, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return MemoryDecision{}, fmt.Errorf("记忆决策为空")
	}
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return MemoryDecision{}, fmt.Errorf("记忆决策 JSON 格式错误")
	}
	jsonText := text[start : end+1]

	var decision MemoryDecision
	if err := json.Unmarshal([]byte(jsonText), &decision); err != nil {
		return MemoryDecision{}, err
	}
	return decision, nil
}

func flattenMessages(messages []models.Message) string {
	var b strings.Builder
	for _, item := range messages {
		role := strings.TrimSpace(item.Role)
		if role == "" {
			role = "unknown"
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(item.Content))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func splitByUnicodeWord(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-'
	})
	return fields
}

func normalizeToken(token string) string {
	normalized := strings.ToLower(strings.TrimSpace(token))
	if normalized == "" {
		return ""
	}
	normalized = nonWordRuneRegex.ReplaceAllString(normalized, "")
	if normalized == "" {
		return ""
	}
	runeCount := len([]rune(normalized))
	if runeCount <= 1 {
		return ""
	}
	return normalized
}
