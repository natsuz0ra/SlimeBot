package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"

	"github.com/go-ego/gse"
)

var defaultMemoryStopWords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "for": {}, "to": {}, "of": {}, "in": {}, "on": {}, "at": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "with": {}, "from": {}, "this": {}, "that": {}, "it": {},
	"你": {}, "我": {}, "他": {}, "她": {}, "它": {}, "我们": {}, "你们": {}, "他们": {}, "以及": {}, "并且": {}, "或者": {},
	"一个": {}, "一些": {}, "可以": {}, "需要": {}, "然后": {}, "就是": {}, "这里": {}, "这个": {}, "那个": {},
}

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

// ShouldCompressContext 根据消息数量判断是否进入记忆压缩策略。
func (m *MemoryService) ShouldCompressContext(sessionID string) (bool, int64, error) {
	total, err := m.repo.CountSessionMessages(sessionID)
	if err != nil {
		return false, 0, err
	}
	return total >= consts.CompressHistoryThreshold, total, nil
}

// DecideMemoryQuery 通过小模型决策当前提问是否需要检索历史记忆。
func (m *MemoryService) DecideMemoryQuery(ctx context.Context, modelConfig ModelRuntimeConfig, userInput string, summary string) (MemoryDecision, error) {
	systemPrompt := `You are a memory retrieval decision engine. Based on the current user input and session summary, decide whether historical memory retrieval is needed to answer.
Return JSON only. Do not output any extra text.
JSON format:
{"need_memory":true/false,"keywords":["keyword1","keyword2"],"reason":"brief reason"}
Requirements:
1. Set need_memory=true only when the request depends on historical facts, preferences, long-term tasks, or cross-session context.
2. Keep 1-8 retrievable keywords in keywords, avoiding stop words.
3. If retrieval is not needed, return an empty keywords array.`

	userPrompt := fmt.Sprintf("User input:\n%s\n\nCurrent session summary:\n%s", strings.TrimSpace(userInput), strings.TrimSpace(summary))
	reply, attempts, elapsed, err := m.chatOnceWithRetry(ctx, modelConfig, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, consts.MemoryDecisionTimeout, "memory_decision")
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
		consts.MemoryDecisionTimeout.Milliseconds(),
	)
	return decision, nil
}

// RetrieveMemories 根据关键词检索跨会话记忆，并可排除当前会话。
func (m *MemoryService) RetrieveMemories(keywords []string, excludeSessionID string, limit int) ([]repositories.SessionMemorySearchHit, error) {
	if limit <= 0 {
		limit = consts.MemorySearchTopK
	}
	return m.repo.SearchMemoriesByKeywords(m.TokenizeKeywords(strings.Join(keywords, " ")), limit, excludeSessionID)
}

// FormatMemoryContext 将摘要与检索命中组织为可直接注入提示词的块结构。
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

// FormatCurrentSessionContext 仅组织当前会话摘要为 memory_context 块，避免引入跨会话检索结果。
func (m *MemoryService) FormatCurrentSessionContext(summary string) string {
	if strings.TrimSpace(summary) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("<current_session_summary>\n")
	b.WriteString(strings.TrimSpace(summary))
	b.WriteString("\n</current_session_summary>")
	return b.String()
}

// QueryForAgent 是 memory 工具入口，返回标准化的检索结果文本。
func (m *MemoryService) QueryForAgent(sessionID string, query string, topK int) (MemoryQueryResult, error) {
	result := MemoryQueryResult{
		Query: strings.TrimSpace(query),
	}
	if result.Query == "" {
		return result, fmt.Errorf("memory_query query cannot be empty")
	}
	if topK <= 0 {
		topK = 3
	}
	if topK > 5 {
		topK = 5
	}

	result.Keywords = m.TokenizeKeywords(result.Query)
	if len(result.Keywords) == 0 {
		result.Output = "<memory_query_result>\nNo retrievable keywords extracted. Please refine the query and try again.\n</memory_query_result>"
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
		b.WriteString("No related memories found.\n")
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

// BuildCompactHistory 返回用于上下文压缩的近期消息切片。
func (m *MemoryService) BuildCompactHistory(sessionID string) ([]models.Message, error) {
	return m.BuildRecentHistory(sessionID, consts.CompactRawHistoryLimit)
}

// BuildRecentHistory 返回用于上下文构建的近期消息切片。
func (m *MemoryService) BuildRecentHistory(sessionID string, limit int) ([]models.Message, error) {
	if limit <= 0 {
		limit = consts.CompressedRecentHistoryLimit
	}
	return m.repo.ListRecentSessionMessages(sessionID, limit)
}

// UpdateSummaryAsync 异步触发摘要更新，并保证同一会话串行执行。
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

// runSummaryWorker 串行消费同会话的摘要更新任务，合并并发触发。
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

// runSummaryOnce 执行一次完整摘要更新：读取近期消息 -> 生成摘要 -> 持久化。
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

	recent, err := m.repo.ListRecentSessionMessages(sessionID, consts.MemorySummaryRecentMessageSize)
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
			consts.MemorySummaryTimeout.Milliseconds(),
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
		consts.MemorySummaryTimeout.Milliseconds(),
		time.Since(startAt).Milliseconds(),
	)
}

// MergeSummary 合并历史摘要与近期消息，生成新的会话记忆摘要。
func (m *MemoryService) MergeSummary(ctx context.Context, modelConfig ModelRuntimeConfig, oldSummary string, recent []models.Message) (string, int, time.Duration, error) {
	systemPrompt := `You are a conversation summarizer. Merge the historical summary and latest conversation snippets into a new high-quality memory summary.
Output requirements:
1. Output summary text only. Do not use markdown headings and do not output JSON.
2. Write in a natural narrative style and allow multiple paragraphs.
3. Include user question time, user asks, and conclusion/decision whenever the timeline is clear from inputs.
4. Keep user preferences, key facts, completed/pending tasks, important constraints, and useful context clues.
5. For older history, merge and compress repetitive details while preserving key turning points.
6. Remove greetings, repeated details, irrelevant tool logs, and branches/options the user did not continue to pursue.
7. If there were multiple options, summarize as "multiple options were considered and user selected X" when helpful.`
	userPrompt := fmt.Sprintf("Historical summary:\n%s\n\nLatest conversation snippets:\n%s", strings.TrimSpace(oldSummary), flattenMessages(recent))

	reply, attempts, elapsed, err := m.chatOnceWithRetry(ctx, modelConfig, []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, consts.MemorySummaryTimeout, "memory_summary")
	if err != nil {
		return "", attempts, elapsed, err
	}

	summary := strings.TrimSpace(reply)
	if summary == "" {
		return "", attempts, elapsed, fmt.Errorf("summary generation returned empty output")
	}
	return summary, attempts, elapsed, nil
}

// chatOnce 以流式接口调用模型并收集完整文本输出。
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

// TokenizeKeywords 将输入分词去重并过滤停用词，产出检索关键词。
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
		if len(result) >= consts.MemoryKeywordMaxCount {
			break
		}
	}
	return result
}

func (m *MemoryService) chatOnceWithRetry(
	parent context.Context,
	modelConfig ModelRuntimeConfig,
	messages []ChatMessage,
	timeout time.Duration,
	stage string,
) (string, int, time.Duration, error) {
	// memory 阶段只允许有限重试，避免后台任务长期阻塞。
	startAt := time.Now()
	var lastErr error
	attempts := 0

	for attempt := 1; attempt <= consts.MemoryCallMaxAttempts; attempt++ {
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

		retryable := attempt < consts.MemoryCallMaxAttempts && isRetryableMemoryError(err) && parent.Err() == nil
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
		case <-time.After(consts.MemoryRetryBackoff):
		}
	}

	return "", attempts, time.Since(startAt), lastErr
}
