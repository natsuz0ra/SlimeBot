package memory

import (
	"context"
	"fmt"
	"log/slog"
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

	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWg     sync.WaitGroup

	segOnce   sync.Once
	segmenter gse.Segmenter
}

func NewMemoryService(store domain.MemoryStore, openai *OpenAIClient) *MemoryService {
	wctx, wcancel := context.WithCancel(context.Background())
	service := &MemoryService{
		store:        store,
		openai:       openai,
		workers:      make(map[string]*memoryWorkerState),
		vectorTopK:   constants.MemorySearchTopK,
		workerCtx:    wctx,
		workerCancel: wcancel,
	}
	service.chatInvoker = service.chatOnce
	return service
}

func (m *MemoryService) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.workerCancel()
	done := make(chan struct{})
	go func() {
		m.workerWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

func (m *MemoryService) WarmupTokenizer() {
	if m == nil {
		return
	}
	m.TokenizeKeywords(" ")
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
	created, err := m.store.CreateSessionMemory(domain.SessionMemoryCreateInput{
		SessionID:          normalizedSessionID,
		Summary:            normalizedSummary,
		Keywords:           keywords,
		SourceMessageCount: int(messageCount),
	})
	if err != nil {
		return false, err
	}
	if m.embedding != nil && m.vectorStore != nil && created != nil {
		_ = upsertMemoryVector(m, context.Background(), created.ID, normalizedSessionID, normalizedSummary, keywords, int(messageCount))
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
	slog.Info("memory_decision_done",
		"need_memory", decision.NeedMemory,
		"keywords", len(decision.Keywords),
		"attempts", attempts,
		"cost_ms", elapsed.Milliseconds(),
		"timeout_ms", constants.MemoryDecisionTimeout.Milliseconds(),
	)
	return decision, nil
}

// RetrieveMemories 根据查询文本检索跨会话记忆，并可排除当前会话。
func (m *MemoryService) RetrieveMemories(ctx context.Context, query string, excludeSessionID string, limit int) ([]domain.SessionMemorySearchHit, error) {
	startAt := time.Now()
	if limit <= 0 {
		limit = constants.MemorySearchTopK
	}
	normalizedKeywords := m.TokenizeKeywords(strings.TrimSpace(query))

	if m.embedding != nil && m.vectorStore != nil {
		hits, err := m.retrieveMemoriesByVector(ctx, query, excludeSessionID, limit)
		if err != nil {
			slog.Warn("memory_vector_retrieve_fallback", "reason", "vector_error", "err", err)
		} else if len(hits) > 0 {
			slog.Info("memory_retrieve",
				"mode", "vector",
				"hit_count", len(hits),
				"keyword_count", len(normalizedKeywords),
				"cost_ms", time.Since(startAt).Milliseconds(),
			)
			return hits, nil
		} else {
			slog.Info("memory_vector_retrieve_fallback",
				"reason", "empty_result",
				"keyword_count", len(normalizedKeywords),
				"cost_ms", time.Since(startAt).Milliseconds(),
			)
		}
	}
	hits, err := m.store.SearchMemoriesByKeywords(normalizedKeywords, limit, excludeSessionID)
	if err != nil {
		return nil, err
	}
	slog.Info("memory_retrieve",
		"mode", "keyword",
		"hit_count", len(hits),
		"keyword_count", len(normalizedKeywords),
		"cost_ms", time.Since(startAt).Milliseconds(),
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
			b.WriteString(fmt.Sprintf("  <memory index=\"%d\" id=\"%s\" session_id=\"%s\" matched=\"%s\">\n", idx+1, hit.Memory.ID, hit.Memory.SessionID, strings.Join(hit.MatchedKeywords, ",")))
			b.WriteString("    ")
			b.WriteString(strings.TrimSpace(hit.Memory.Summary))
			b.WriteString("\n  </memory>\n")
		}
		b.WriteString("</retrieved_memories>")
	}
	return strings.TrimSpace(b.String())
}

func FormatMemoriesListXMLWithBudget(memories []domain.SessionMemory, maxRunes int) string {
	if len(memories) == 0 {
		return ""
	}
	if maxRunes <= 0 {
		return ""
	}
	prefix := "<memories>\n"
	suffix := "</memories>"
	used := len([]rune(prefix)) + len([]rune(suffix))
	if used > maxRunes {
		return ""
	}
	var b strings.Builder
	b.WriteString(prefix)
	for _, mem := range memories {
		entry := fmt.Sprintf(`  <memory id="%s" created="%s" updated="%s">%s</memory>`+"\n",
			mem.ID,
			mem.CreatedAt.Format(time.RFC3339),
			mem.UpdatedAt.Format(time.RFC3339),
			strings.TrimSpace(mem.Summary))
		entryRunes := len([]rune(entry))
		if used+entryRunes > maxRunes {
			break
		}
		b.WriteString(entry)
		used += entryRunes
	}
	b.WriteString(suffix)
	return b.String()
}

// QueryForAgent 是 memory 工具入口，返回标准化的检索结果文本。
func (m *MemoryService) QueryForAgent(ctx context.Context, sessionID string, query string, topK int) (MemoryQueryResult, error) {
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

	hits, err := m.RetrieveMemories(ctx, result.Query, strings.TrimSpace(sessionID), topK)
	if err != nil {
		return result, err
	}
	result.Keywords = m.TokenizeKeywords(result.Query)
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
		b.WriteString(fmt.Sprintf("- [%d] id=%s session_id=%s score=%.1f matched=%s created_at=%s updated_at=%s\n", idx+1, hit.Memory.ID, hit.Memory.SessionID, hit.Score, strings.Join(hit.MatchedKeywords, ","), hit.Memory.CreatedAt.Format(time.RFC3339), hit.Memory.UpdatedAt.Format(time.RFC3339)))
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

func (m *MemoryService) UpdateSummaryAsync(sessionID string, rawSummary string) {
	updateSummaryAsyncImpl(m, sessionID, rawSummary)
}

func (m *MemoryService) BuildSessionMemoryContextForPrompt(ctx context.Context, sessionID string, history []domain.Message) string {
	if m == nil {
		return ""
	}
	return m.buildSessionMemoryContextForPrompt(ctx, sessionID, history)
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
		slog.Warn(stage+"_attempt_failed",
			"attempt", attempt,
			"timeout_ms", timeout.Milliseconds(),
			"retryable", retryable,
			"err_class", classifyMemoryError(err),
			"err", err,
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

func intersectKeywordSlices(left []string, right []string) []string {
	return intersectKeywordSlicesImpl(left, right)
}
