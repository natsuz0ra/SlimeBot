package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
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
	memoryDecisionTimeout          = 20 * time.Second
	memorySummaryTimeout           = 45 * time.Second
	memoryCallMaxAttempts          = 2
	memoryRetryBackoff             = 350 * time.Millisecond
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

type MemoryQueryResult struct {
	Query    string
	Keywords []string
	Hits     []repositories.SessionMemorySearchHit
	Output   string
}

type MemoryService struct {
	repo        *repositories.Repository
	openai      *OpenAIClient
	chatInvoker func(context.Context, ModelRuntimeConfig, []ChatMessage) (string, error)

	workerMu sync.Mutex
	workers  map[string]*memoryWorkerState

	segOnce   sync.Once
	segmenter gse.Segmenter
}

type memoryWorkerState struct {
	running bool
	pending bool
}

func NewMemoryService(repo *repositories.Repository, openai *OpenAIClient) *MemoryService {
	service := &MemoryService{
		repo:    repo,
		openai:  openai,
		workers: make(map[string]*memoryWorkerState),
	}
	service.chatInvoker = service.chatOnce
	return service
}

func (m *MemoryService) ShouldCompressContext(sessionID string) (bool, int64, error) {
	total, err := m.repo.CountSessionMessages(sessionID)
	if err != nil {
		return false, 0, err
	}
	return total >= compressHistoryThreshold, total, nil
}

func (m *MemoryService) DecideMemoryQuery(ctx context.Context, modelConfig ModelRuntimeConfig, userInput string, summary string) (MemoryDecision, error) {
	systemPrompt := `你是“记忆检索决策器”。请根据用户当前输入和会话摘要，判断是否需要检索历史记忆来回答问题。
仅返回 JSON，不要输出任何额外文本。
JSON 格式：
{"need_memory":true/false,"keywords":["关键词1","关键词2"],"reason":"简短原因"}
要求：
1. 只有在用户问题依赖历史事实、偏好、长期任务或跨会话信息时，need_memory=true。
2. keywords 仅保留 1~8 个可检索关键词，避免停用词。
3. 若不需要检索，keywords 返回空数组。`

	userPrompt := fmt.Sprintf("用户输入：\n%s\n\n当前会话摘要：\n%s", strings.TrimSpace(userInput), strings.TrimSpace(summary))
	reply, attempts, elapsed, err := m.chatOnceWithRetry(ctx, modelConfig, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, memoryDecisionTimeout, "memory_decision")
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
	log.Printf(
		"memory_decision_done need_memory=%t keywords=%d attempts=%d cost_ms=%d timeout_ms=%d",
		decision.NeedMemory,
		len(decision.Keywords),
		attempts,
		elapsed.Milliseconds(),
		memoryDecisionTimeout.Milliseconds(),
	)
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

func (m *MemoryService) QueryForAgent(sessionID string, query string, topK int) (MemoryQueryResult, error) {
	result := MemoryQueryResult{
		Query: strings.TrimSpace(query),
	}
	if result.Query == "" {
		return result, fmt.Errorf("memory_query 参数 query 不能为空")
	}
	if topK <= 0 {
		topK = 3
	}
	if topK > 5 {
		topK = 5
	}

	result.Keywords = m.TokenizeKeywords(result.Query)
	if len(result.Keywords) == 0 {
		result.Output = "<memory_query_result>\n未提取到可检索关键词，请改写 query 后重试。\n</memory_query_result>"
		return result, nil
	}

	hits, err := m.RetrieveMemories(result.Keywords, strings.TrimSpace(sessionID), topK)
	if err != nil {
		return result, err
	}
	result.Hits = hits
	result.Output = m.buildMemoryQueryOutput(result.Query, result.Keywords, hits)
	return result, nil
}

func (m *MemoryService) buildMemoryQueryOutput(query string, keywords []string, hits []repositories.SessionMemorySearchHit) string {
	var b strings.Builder
	b.WriteString("<memory_query_result>\n")
	b.WriteString("query: ")
	b.WriteString(strings.TrimSpace(query))
	b.WriteString("\n")
	b.WriteString("keywords: ")
	if len(keywords) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(keywords, ", "))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("hit_count: %d\n", len(hits)))
	if len(hits) == 0 {
		b.WriteString("未检索到相关记忆。\n")
		b.WriteString("</memory_query_result>")
		return b.String()
	}
	for idx, hit := range hits {
		b.WriteString(fmt.Sprintf("- [%d] session_id=%s score=%.1f matched=%s\n", idx+1, hit.Memory.SessionID, hit.Score, strings.Join(hit.MatchedKeywords, ",")))
		b.WriteString("  summary: ")
		b.WriteString(strings.TrimSpace(hit.Memory.Summary))
		b.WriteString("\n")
	}
	b.WriteString("</memory_query_result>")
	return strings.TrimSpace(b.String())
}

func (m *MemoryService) BuildCompactHistory(sessionID string) ([]models.Message, error) {
	return m.repo.ListRecentSessionMessages(sessionID, compactRawHistoryLimit)
}

func (m *MemoryService) UpdateSummaryAsync(modelConfig ModelRuntimeConfig, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	m.workerMu.Lock()
	state := m.workers[sessionID]
	if state == nil {
		state = &memoryWorkerState{}
		m.workers[sessionID] = state
	}
	if state.running {
		state.pending = true
		m.workerMu.Unlock()
		log.Printf("memory_summary_queued session=%s reason=worker_running", sessionID)
		return
	}
	state.running = true
	m.workerMu.Unlock()

	go m.runSummaryWorker(modelConfig, sessionID)
}

func (m *MemoryService) runSummaryWorker(modelConfig ModelRuntimeConfig, sessionID string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("memory_summary_panic session=%s recovered=%v", sessionID, recovered)
		}
		m.workerMu.Lock()
		delete(m.workers, sessionID)
		m.workerMu.Unlock()
	}()

	for {
		m.runSummaryOnce(modelConfig, sessionID)

		m.workerMu.Lock()
		state, ok := m.workers[sessionID]
		if !ok {
			m.workerMu.Unlock()
			return
		}
		if state.pending {
			state.pending = false
			m.workerMu.Unlock()
			log.Printf("memory_summary_worker_continue session=%s reason=pending_trigger", sessionID)
			continue
		}
		m.workerMu.Unlock()
		return
	}
}

func (m *MemoryService) runSummaryOnce(modelConfig ModelRuntimeConfig, sessionID string) {
	startAt := time.Now()

	ctx := context.Background()
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

	mergedSummary, attempts, summaryCost, err := m.MergeSummary(ctx, modelConfig, oldSummary, recent)
	if err != nil {
		log.Printf(
			"memory_summary_skip session=%s reason=merge_failed attempts=%d cost_ms=%d timeout_ms=%d err_class=%s err=%v",
			sessionID,
			attempts,
			summaryCost.Milliseconds(),
			memorySummaryTimeout.Milliseconds(),
			classifyMemoryError(err),
			err,
		)
		return
	}
	keywords := m.TokenizeKeywords(mergedSummary + "\n" + flattenMessages(recent))
	updated, err := m.repo.UpsertSessionMemoryIfNewer(repositories.SessionMemoryUpsertInput{
		SessionID:          sessionID,
		Summary:            mergedSummary,
		Keywords:           keywords,
		SourceMessageCount: int(totalMessages),
	})
	if err != nil {
		log.Printf("memory_summary_skip session=%s reason=upsert_failed err=%v", sessionID, err)
		return
	}
	if !updated {
		log.Printf("memory_summary_skip session=%s reason=stale_write source_message_count=%d", sessionID, totalMessages)
		return
	}

	log.Printf(
		"memory_summary_updated session=%s total_messages=%d keywords=%d attempts=%d summary_cost_ms=%d timeout_ms=%d total_cost_ms=%d",
		sessionID,
		totalMessages,
		len(keywords),
		attempts,
		summaryCost.Milliseconds(),
		memorySummaryTimeout.Milliseconds(),
		time.Since(startAt).Milliseconds(),
	)
}

func (m *MemoryService) MergeSummary(ctx context.Context, modelConfig ModelRuntimeConfig, oldSummary string, recent []models.Message) (string, int, time.Duration, error) {
	systemPrompt := `你是会话摘要器。请将“历史摘要”和“最新对话片段”融合成新的高质量记忆摘要。
输出要求：
1. 只输出摘要正文，不要使用 markdown 标题，不要输出 JSON。
2. 保留：用户偏好、关键事实、已完成/待完成任务、重要约束、上下文线索。
3. 删除：寒暄、重复信息、无关工具日志。
4. 摘要尽量精炼，但不要丢失关键信息。`
	userPrompt := fmt.Sprintf("历史摘要：\n%s\n\n最新对话片段：\n%s", strings.TrimSpace(oldSummary), flattenMessages(recent))

	reply, attempts, elapsed, err := m.chatOnceWithRetry(ctx, modelConfig, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, memorySummaryTimeout, "memory_summary")
	if err != nil {
		return "", attempts, elapsed, err
	}

	summary := strings.TrimSpace(reply)
	if summary == "" {
		return "", attempts, elapsed, fmt.Errorf("摘要生成为空")
	}
	return summary, attempts, elapsed, nil
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

func (m *MemoryService) chatOnceWithRetry(
	parent context.Context,
	modelConfig ModelRuntimeConfig,
	messages []ChatMessage,
	timeout time.Duration,
	stage string,
) (string, int, time.Duration, error) {
	startAt := time.Now()
	var lastErr error
	attempts := 0

	for attempt := 1; attempt <= memoryCallMaxAttempts; attempt++ {
		attempts = attempt
		attemptCtx, cancel := context.WithTimeout(parent, timeout)
		invoker := m.chatInvoker
		if invoker == nil {
			invoker = m.chatOnce
		}
		reply, err := invoker(attemptCtx, modelConfig, messages)
		cancel()
		if err == nil {
			return reply, attempts, time.Since(startAt), nil
		}
		lastErr = err

		retryable := attempt < memoryCallMaxAttempts && isRetryableMemoryError(err) && parent.Err() == nil
		log.Printf(
			"%s_attempt_failed attempt=%d timeout_ms=%d retryable=%t err_class=%s err=%v",
			stage,
			attempt,
			timeout.Milliseconds(),
			retryable,
			classifyMemoryError(err),
			err,
		)
		if !retryable {
			break
		}
		select {
		case <-parent.Done():
			return "", attempts, time.Since(startAt), parent.Err()
		case <-time.After(memoryRetryBackoff):
		}
	}

	return "", attempts, time.Since(startAt), lastErr
}

func isRetryableMemoryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	text := strings.ToLower(err.Error())
	return strings.Contains(text, "deadline exceeded") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "i/o timeout") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "eof")
}

func classifyMemoryError(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}

	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "deadline exceeded"), strings.Contains(text, "timeout"), strings.Contains(text, "i/o timeout"):
		return "deadline_exceeded"
	case strings.Contains(text, "connection reset"), strings.Contains(text, "broken pipe"), strings.Contains(text, "eof"):
		return "network_error"
	default:
		return "unknown"
	}
}
