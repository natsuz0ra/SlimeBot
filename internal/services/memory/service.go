package memory

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"

	"github.com/go-ego/gse"
)

type MemoryDecision struct {
	NeedMemory bool     `json:"need_memory"`
	Keywords   []string `json:"keywords"`
	Reason     string   `json:"reason"`
}

type MemoryQueryResult struct {
	Query    string
	Keywords []string
	Hits     []domain.SessionMemorySearchHit
	Output   string
}

type MemoryService struct {
	store       domain.MemoryStore
	openai      *OpenAIClient
	chatInvoker func(context.Context, ModelRuntimeConfig, []ChatMessage) (string, error)
	embedding   EmbeddingService
	vectorStore domain.MemoryVectorStore
	vectorTopK  int

	workerMu sync.Mutex
	workers  map[string]*memoryWorkerState

	segOnce   sync.Once
	segmenter gse.Segmenter
}

type memoryWorkerState struct {
	running bool
	pending bool
}

func NewMemoryService(store domain.MemoryStore, openai *OpenAIClient) *MemoryService {
	service := &MemoryService{
		store:      store,
		openai:     openai,
		workers:    make(map[string]*memoryWorkerState),
		vectorTopK: constants.MemorySearchTopK,
	}
	service.chatInvoker = service.chatOnce
	return service
}

func (m *MemoryService) SetEmbeddingService(embedding EmbeddingService) {
	m.embedding = embedding
}

func (m *MemoryService) SetVectorStore(store domain.MemoryVectorStore) {
	m.vectorStore = store
}

func (m *MemoryService) SetVectorSearchTopK(topK int) {
	if topK <= 0 {
		return
	}
	m.vectorTopK = topK
}

// PersistSessionSummary 统一处理会话摘要持久化：摘要+关键词写库，并在启用时写入向量库。
func (m *MemoryService) PersistSessionSummary(sessionID string, summary string) (bool, error) {
	normalizedSessionID := strings.TrimSpace(sessionID)
	normalizedSummary := strings.TrimSpace(summary)
	if normalizedSessionID == "" {
		return false, fmt.Errorf("session_id cannot be empty")
	}
	if normalizedSummary == "" {
		return false, fmt.Errorf("summary cannot be empty")
	}

	messageCount, err := m.store.CountSessionMessages(normalizedSessionID)
	if err != nil {
		return false, err
	}
	keywords := m.TokenizeKeywords(normalizedSummary)
	updated, err := m.store.UpsertSessionMemoryIfNewer(domain.SessionMemoryUpsertInput{
		SessionID:          normalizedSessionID,
		Summary:            normalizedSummary,
		Keywords:           keywords,
		SourceMessageCount: int(messageCount),
	})
	if err != nil {
		return false, err
	}
	if !updated {
		return false, nil
	}

	if m.embedding != nil && m.vectorStore != nil {
		if err := m.upsertSessionMemoryVector(context.Background(), normalizedSessionID, normalizedSummary, keywords, int(messageCount)); err != nil {
			// 向量写入失败不阻断主流程，保障摘要可持续落库。
			// 详细失败日志已在 upsertSessionMemoryVector 内记录，这里避免重复打印。
		}
	}
	return true, nil
}

// ShouldCompressContext 根据消息数量判断是否进入记忆压缩策略。
func (m *MemoryService) ShouldCompressContext(sessionID string) (bool, int64, error) {
	total, err := m.store.CountSessionMessages(sessionID)
	if err != nil {
		return false, 0, err
	}
	return total >= constants.CompressHistoryThreshold, total, nil
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
	}, constants.MemoryDecisionTimeout, "memory_decision")
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
		constants.MemoryDecisionTimeout.Milliseconds(),
	)
	return decision, nil
}

// RetrieveMemories 根据关键词检索跨会话记忆，并可排除当前会话。
func (m *MemoryService) RetrieveMemories(keywords []string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	startAt := time.Now()
	if limit <= 0 {
		limit = constants.MemorySearchTopK
	}
	normalizedKeywords := m.TokenizeKeywords(strings.Join(keywords, " "))

	if m.embedding != nil && m.vectorStore != nil {
		hits, err := m.retrieveMemoriesByVector(normalizedKeywords, excludeSessionID, limit)
		if err != nil {
			log.Printf("memory_vector_retrieve_fallback reason=vector_error err=%v", err)
		} else if len(hits) > 0 {
			log.Printf(
				"memory_retrieve mode=vector hit_count=%d keyword_count=%d cost_ms=%d",
				len(hits),
				len(normalizedKeywords),
				time.Since(startAt).Milliseconds(),
			)
			return hits, nil
		} else {
			log.Printf(
				"memory_vector_retrieve_fallback reason=empty_result keyword_count=%d cost_ms=%d",
				len(normalizedKeywords),
				time.Since(startAt).Milliseconds(),
			)
		}
	}
	hits, err := m.store.SearchMemoriesByKeywords(normalizedKeywords, limit, excludeSessionID)
	if err != nil {
		return nil, err
	}
	log.Printf(
		"memory_retrieve mode=keyword hit_count=%d keyword_count=%d cost_ms=%d",
		len(hits),
		len(normalizedKeywords),
		time.Since(startAt).Milliseconds(),
	)
	return hits, nil
}

// FormatMemoryContext 将摘要与检索命中组织为可直接注入提示词的块结构。
func (m *MemoryService) FormatMemoryContext(summary string, hits []domain.SessionMemorySearchHit) string {
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

func (m *MemoryService) buildMemoryQueryOutput(query string, keywords []string, hits []domain.SessionMemorySearchHit) string {
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
func (m *MemoryService) BuildCompactHistory(sessionID string) ([]domain.Message, error) {
	return m.BuildRecentHistory(sessionID, constants.CompactRawHistoryLimit)
}

// BuildRecentHistory 返回用于上下文构建的近期消息切片。
func (m *MemoryService) BuildRecentHistory(sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = constants.CompressedRecentHistoryLimit
	}
	return m.store.ListRecentSessionMessages(sessionID, limit)
}

// UpdateSummaryAsync 异步触发摘要更新，并保证同一会话串行执行。
func (m *MemoryService) UpdateSummaryAsync(modelConfig ModelRuntimeConfig, sessionID string) {
	updateSummaryAsyncImpl(m, modelConfig, sessionID)
}

// runSummaryWorker 串行消费同会话的摘要更新任务，合并并发触发。
func (m *MemoryService) runSummaryWorker(modelConfig ModelRuntimeConfig, sessionID string) {
	runSummaryWorkerImpl(m, modelConfig, sessionID)
}

// runSummaryOnce 执行一次完整摘要更新：读取近期消息 -> 生成摘要 -> 持久化。
func (m *MemoryService) runSummaryOnce(modelConfig ModelRuntimeConfig, sessionID string) {
	runSummaryOnceImpl(m, modelConfig, sessionID)
}

// MergeSummary 合并历史摘要与近期消息，生成新的会话记忆摘要。
func (m *MemoryService) MergeSummary(ctx context.Context, modelConfig ModelRuntimeConfig, oldSummary string, recent []domain.Message) (string, int, time.Duration, error) {
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
	}, constants.MemorySummaryTimeout, "memory_summary")
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
	return tokenizeKeywordsImpl(m, text)
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

	for attempt := 1; attempt <= constants.MemoryCallMaxAttempts; attempt++ {
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

		retryable := attempt < constants.MemoryCallMaxAttempts && isRetryableMemoryError(err) && parent.Err() == nil
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
		case <-time.After(constants.MemoryRetryBackoff):
		}
	}

	return "", attempts, time.Since(startAt), lastErr
}

func (m *MemoryService) retrieveMemoriesByVector(keywords []string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	return retrieveMemoriesByVectorImpl(m, keywords, excludeSessionID, limit)
}

func (m *MemoryService) upsertSessionMemoryVector(ctx context.Context, sessionID string, summary string, keywords []string, messageCount int) error {
	return upsertSessionMemoryVectorImpl(m, ctx, sessionID, summary, keywords, messageCount)
}

func intersectKeywordSlices(left []string, right []string) []string {
	return intersectKeywordSlicesImpl(left, right)
}
