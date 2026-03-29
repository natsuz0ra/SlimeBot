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
	reopenTopicWindow = 2 * time.Hour
	splitSoftWindow   = 15 * time.Minute
	splitHardWindow   = 6 * time.Hour
)

type MemorySearchHit struct {
	Kind     string
	ID       string
	Title    string
	Summary  string
	Score    float64
	Status   string
	Keywords []string
}

type MemoryQueryResult struct {
	Query    string
	Keywords []string
	Hits     []MemorySearchHit
	Output   string
}

type queuedTurnMemory struct {
	assistantMessageID string
	rawPayload         string
}

type memoryWorkerState struct {
	running bool
	pending []queuedTurnMemory
}

type MemoryService struct {
	store       domain.MemoryStore
	openai      *oaisvc.OpenAIClient
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
	return &MemoryService{
		store:        store,
		openai:       openai,
		vectorTopK:   constants.MemorySearchTopK,
		workers:      make(map[string]*memoryWorkerState),
		workerCtx:    wctx,
		workerCancel: wcancel,
	}
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

func (m *MemoryService) TokenizeKeywords(text string) []string {
	return tokenizeKeywordsImpl(m, text)
}

func (m *MemoryService) EnqueueTurnMemory(sessionID, assistantMessageID, rawMemoryPayload string) {
	enqueueTurnMemoryImpl(m, sessionID, assistantMessageID, rawMemoryPayload)
}

func (m *MemoryService) BuildMemoryContext(ctx context.Context, sessionID string, history []domain.Message) string {
	if m == nil {
		return ""
	}
	return m.buildMemoryContext(ctx, sessionID, history)
}

func (m *MemoryService) BuildSessionMemoryContextForPrompt(ctx context.Context, sessionID string, history []domain.Message) string {
	return m.BuildMemoryContext(ctx, sessionID, history)
}

func (m *MemoryService) BuildRecentHistory(sessionID string, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = constants.CompressedRecentHistoryLimit
	}
	return m.store.ListRecentSessionMessages(context.Background(), sessionID, limit)
}

func (m *MemoryService) QueryForAgent(ctx context.Context, sessionID string, query string, topK int) (MemoryQueryResult, error) {
	result := MemoryQueryResult{Query: strings.TrimSpace(query)}
	if result.Query == "" {
		return result, fmt.Errorf("memory_query query cannot be empty")
	}
	if topK <= 0 {
		topK = constants.MemoryToolDefaultTopK
	}
	result.Keywords = m.TokenizeKeywords(result.Query)

	episodeHits, err := m.RetrieveRelevantEpisodes(ctx, "", result.Query, 0, 0, topK)
	if err != nil {
		return result, err
	}
	stickyHits, err := m.store.SearchStickyMemories(ctx, "", result.Query, topK, time.Now())
	if err != nil {
		return result, err
	}

	result.Hits = mergeQueryHits(episodeHits, stickyHits, topK)
	result.Output = buildMemoryQueryOutput(result.Query, result.Keywords, result.Hits)
	return result, nil
}

func mergeQueryHits(episodes []domain.EpisodeMemorySearchHit, sticky []domain.StickyMemorySearchHit, topK int) []MemorySearchHit {
	hits := make([]MemorySearchHit, 0, len(episodes)+len(sticky))
	for _, item := range episodes {
		hits = append(hits, MemorySearchHit{
			Kind:     "episode",
			ID:       item.Episode.ID,
			Title:    item.Episode.Title,
			Summary:  item.Episode.Summary,
			Score:    item.Score,
			Status:   item.Episode.State,
			Keywords: decodeKeywordsJSON(item.Episode.KeywordsJSON),
		})
	}
	for _, item := range sticky {
		hits = append(hits, MemorySearchHit{
			Kind:     "sticky",
			ID:       item.Memory.ID,
			Title:    item.Memory.Key,
			Summary:  item.Memory.Summary,
			Score:    item.Score,
			Status:   item.Memory.Status,
			Keywords: []string{item.Memory.Kind, item.Memory.Key},
		})
	}
	sortMemoryHits(hits)
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits
}

func sortMemoryHits(hits []MemorySearchHit) {
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
}

func buildMemoryQueryOutput(query string, keywords []string, hits []MemorySearchHit) string {
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
	b.WriteString(fmt.Sprintf("\nhit_count: %d\n", len(hits)))
	if len(hits) == 0 {
		b.WriteString("No related memories found.\n</memory_query_result>")
		return b.String()
	}
	for idx, item := range hits {
		b.WriteString(fmt.Sprintf("- [%d] kind=%s id=%s status=%s score=%.2f title=%s\n", idx+1, item.Kind, item.ID, item.Status, item.Score, item.Title))
		b.WriteString("  summary: ")
		b.WriteString(strings.TrimSpace(item.Summary))
		b.WriteString("\n")
	}
	b.WriteString("</memory_query_result>")
	return strings.TrimSpace(b.String())
}

func enqueueLog(sessionID string, rawPayload string) {
	slog.Info("memory_turn_enqueued", "session", sessionID, "payload_len", len(strings.TrimSpace(rawPayload)))
}
