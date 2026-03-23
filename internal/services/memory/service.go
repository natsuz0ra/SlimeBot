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
	embsvc "slimebot/internal/services/embedding"
	oaisvc "slimebot/internal/services/openai"

	"github.com/go-ego/gse"
)

const (
	minFactConfidence   = 0.55
	defaultTaskTTL      = 24 * time.Hour
	promptFactMaxCount  = 24
	promptGroupOverhead = 64
)

type MemoryDecision struct {
	NeedMemory bool     `json:"need_memory"`
	Keywords   []string `json:"keywords"`
	Reason     string   `json:"reason"`
}

type MemoryQueryResult struct {
	Query    string
	Keywords []string
	Hits     []domain.MemoryFactSearchHit
	Output   string
}

type MemoryService struct {
	store       domain.MemoryStore
	openai      *oaisvc.OpenAIClient
	chatInvoker func(context.Context, oaisvc.ModelRuntimeConfig, []oaisvc.ChatMessage) (string, error)
	embedding   embsvc.EmbeddingService
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

func NewMemoryService(store domain.MemoryStore, openai *oaisvc.OpenAIClient) *MemoryService {
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

func (m *MemoryService) SetEmbeddingService(embedding embsvc.EmbeddingService) {
	m.embedding = embedding
}

func (m *MemoryService) SetVectorStore(store domain.MemoryVectorStore) {
	m.vectorStore = store
}

func (m *MemoryService) SetVectorSearchTopK(topK int) {
	if topK > 0 {
		m.vectorTopK = topK
	}
}

func (m *MemoryService) WarmupTokenizer() {
	if m != nil {
		m.TokenizeKeywords(" ")
	}
}

func (m *MemoryService) RetrieveMemories(ctx context.Context, query string, excludeSessionID string, limit int) ([]domain.MemoryFactSearchHit, error) {
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
			slog.Info("memory_retrieve", "mode", "vector", "hit_count", len(hits), "keyword_count", len(normalizedKeywords), "cost_ms", time.Since(startAt).Milliseconds())
			return hits, nil
		}
	}

	hits, err := m.store.SearchMemoryFacts(domain.MemoryFactSearchInput{
		Query:          query,
		Limit:          limit,
		ExcludeSession: strings.TrimSpace(excludeSessionID),
		Now:            time.Now(),
	})
	if err != nil {
		return nil, err
	}
	slog.Info("memory_retrieve", "mode", "structured", "hit_count", len(hits), "keyword_count", len(normalizedKeywords), "cost_ms", time.Since(startAt).Milliseconds())
	return hits, nil
}

func (m *MemoryService) QueryForAgent(ctx context.Context, sessionID string, query string, topK int) (MemoryQueryResult, error) {
	result := MemoryQueryResult{Query: strings.TrimSpace(query)}
	if result.Query == "" {
		return result, fmt.Errorf("memory_query query cannot be empty")
	}
	if topK <= 0 {
		topK = 3
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

func (m *MemoryService) buildMemoryQueryOutput(query string, keywords []string, hits []domain.MemoryFactSearchHit) string {
	var b strings.Builder
	b.WriteString("<memory_query_result>\n")
	b.WriteString("query: ")
	b.WriteString(strings.TrimSpace(query))
	b.WriteString("\nkeywords: ")
	if len(keywords) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(keywords, ", "))
	}
	b.WriteString("\nhit_count: ")
	b.WriteString(fmt.Sprintf("%d\n", len(hits)))
	if len(hits) == 0 {
		b.WriteString("No related memories found.\n</memory_query_result>")
		return b.String()
	}
	for idx, hit := range hits {
		b.WriteString(fmt.Sprintf("- [%d] id=%s session_id=%s type=%s status=%s score=%.1f confidence=%.2f matched=%s updated_at=%s\n",
			idx+1,
			hit.Fact.ID,
			hit.Fact.SessionID,
			hit.Fact.MemoryType,
			hit.Fact.Status,
			hit.Score,
			hit.Fact.Confidence,
			strings.Join(hit.MatchedKeywords, ","),
			hit.Fact.UpdatedAt.Format(time.RFC3339),
		))
		b.WriteString("  summary: ")
		b.WriteString(strings.TrimSpace(hit.Fact.Summary))
		b.WriteString("\n")
	}
	b.WriteString("</memory_query_result>")
	return strings.TrimSpace(b.String())
}

func (m *MemoryService) BuildRecentHistory(sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = constants.CompressedRecentHistoryLimit
	}
	return m.store.ListRecentSessionMessages(context.Background(), sessionID, limit)
}

func (m *MemoryService) TokenizeKeywords(text string) []string {
	return tokenizeKeywordsImpl(m, text)
}

func (m *MemoryService) SyncFactsAsync(sessionID string, rawFacts string) {
	updateSummaryAsyncImpl(m, sessionID, rawFacts)
}

func (m *MemoryService) UpdateSummaryAsync(sessionID string, rawSummary string) {
	m.SyncFactsAsync(sessionID, rawSummary)
}

func (m *MemoryService) BuildSessionMemoryContextForPrompt(ctx context.Context, sessionID string, history []domain.Message) string {
	if m == nil {
		return ""
	}
	return m.buildSessionMemoryContextForPrompt(ctx, sessionID, history)
}

func (m *MemoryService) chatOnce(ctx context.Context, modelConfig oaisvc.ModelRuntimeConfig, messages []oaisvc.ChatMessage) (string, error) {
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

func (m *MemoryService) chatOnceWithRetry(
	parent context.Context,
	modelConfig oaisvc.ModelRuntimeConfig,
	messages []oaisvc.ChatMessage,
	timeout time.Duration,
	stage string,
) (string, int, time.Duration, error) {
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
		slog.Warn(stage+"_attempt_failed", "attempt", attempt, "timeout_ms", timeout.Milliseconds(), "retryable", retryable, "err_class", classifyMemoryError(err), "err", err)
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
