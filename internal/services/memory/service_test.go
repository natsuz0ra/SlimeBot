package memory

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/domain"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/repositories"
	oaisvc "slimebot/internal/services/openai"
)

func TestParseMemoryDecision_WithCodeFence(t *testing.T) {
	raw := "```json\n{\"need_memory\":true,\"keywords\":[\"golang\",\"rag\"],\"reason\":\"需要历史信息\"}\n```"
	decision, err := parseMemoryDecision(raw)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !decision.NeedMemory {
		t.Fatal("expected need_memory=true")
	}
	if len(decision.Keywords) != 2 {
		t.Fatalf("unexpected keywords length: %d", len(decision.Keywords))
	}
}

func TestMemoryServiceTokenizeKeywords_Multilingual(t *testing.T) {
	svc := NewMemoryService(nil, nil)
	words := svc.TokenizeKeywords("请帮我优化 Golang RAG memory retrieval，在 Windows 上运行")
	if len(words) == 0 {
		t.Fatal("expected non-empty keywords")
	}
	joined := strings.Join(words, " ")
	if !strings.Contains(joined, "golang") {
		t.Fatalf("expected token list to contain golang, got %v", words)
	}
}

func TestMemoryServiceShouldCompressContext(t *testing.T) {
	repo := newTestRepo(t)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < constants.CompressHistoryThreshold; i++ {
		if _, err := addMessage(t, repo, sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	count, err := repo.CountSessionMessages(sessionID)
	if err != nil {
		t.Fatalf("count messages failed: %v", err)
	}
	if count != constants.CompressHistoryThreshold {
		t.Fatalf("unexpected count: %d", count)
	}
}

func TestMemoryServiceRetrieveMemoriesRanking(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	if err := upsertSessionMemory(t, repo, domain.SessionMemoryUpsertInput{
		SessionID:          "s1",
		Summary:            "鐢ㄦ埛鍠滄 golang 涓?rag锛屽叧娉?token 鎴愭湰",
		Keywords:           []string{"golang", "rag", "token"},
		SourceMessageCount: 20,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := upsertSessionMemory(t, repo, domain.SessionMemoryUpsertInput{
		SessionID:          "s2",
		Summary:            "鐢ㄦ埛鍙彁鍒颁簡 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 10,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}

	hits, err := svc.RetrieveMemories(context.Background(), "golang rag", "", 5)
	if err != nil {
		t.Fatalf("retrieve memories failed: %v", err)
	}
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	if hits[0].Memory.SessionID != "s1" {
		t.Fatalf("expected s1 ranked first, got %s", hits[0].Memory.SessionID)
	}
}

func TestMemoryServiceUpdateSummaryAsync_NonBlocking(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 3; i++ {
		if _, err := addMessage(t, repo, sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	start := time.Now()
	svc.UpdateSummaryAsync(sessionID, `{"ops":[{"action":"create","content":"t"}]}`)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected async call to return quickly, elapsed=%s", elapsed)
	}
}

func TestMemoryServiceUpdateSummaryAsync_EventuallyPersistsSummary(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	if _, err := addMessage(t, repo, sessionID, "user", "璇疯浣忔垜鍠滄 Go"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}
	if _, err := addMessage(t, repo, sessionID, "assistant", "好的，我会记住"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	svc.UpdateSummaryAsync(sessionID, `{"ops":[{"action":"create","content":"鐢ㄦ埛鍋忓ソ Go锛屽苟甯屾湜浼氳瘽瀛樻。寮傛鎵ц"}]}`)

	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		item, getErr := repo.GetSessionMemory(context.Background(), sessionID)
		if getErr != nil {
			t.Fatalf("get session memory failed: %v", getErr)
		}
		if item != nil && strings.Contains(item.Summary, "寮傛鎵ц") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected async summary to be eventually persisted")
}

func TestMemoryServiceChatOnceWithRetry_TimeoutThenSuccess(t *testing.T) {
	svc := NewMemoryService(nil, nil)
	callCount := int32(0)
	svc.chatInvoker = func(_ context.Context, _ oaisvc.ModelRuntimeConfig, _ []oaisvc.ChatMessage) (string, error) {
		attempt := atomic.AddInt32(&callCount, 1)
		if attempt == 1 {
			return "", context.DeadlineExceeded
		}
		return "{\"need_memory\":true,\"keywords\":[\"slimebot\"],\"reason\":\"ok\"}", nil
	}

	reply, attempts, _, err := svc.chatOnceWithRetry(
		context.Background(),
		oaisvc.ModelRuntimeConfig{},
		[]oaisvc.ChatMessage{{Role: "user", Content: "test"}},
		1*time.Second,
		"memory_decision",
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.TrimSpace(reply) == "" {
		t.Fatal("expected non-empty reply")
	}
	if attempts != 2 {
		t.Fatalf("expected attempts=2, got %d", attempts)
	}
}

func TestMemoryServiceUpdateSummaryAsync_SerializesSameSession(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 4; i++ {
		if _, err := addMessage(t, repo, sessionID, "user", fmt.Sprintf("hello-%d", i)); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	svc.UpdateSummaryAsync(sessionID, `{"ops":[{"action":"create","content":"a"}]}`)
	svc.UpdateSummaryAsync(sessionID, `{"ops":[{"action":"create","content":"b"}]}`)
	svc.UpdateSummaryAsync(sessionID, `{"ops":[{"action":"create","content":"c"}]}`)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		all, _ := repo.ListActiveSessionMemories(context.Background(), sessionID)
		if len(all) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	all, err := repo.ListActiveSessionMemories(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("list memories failed: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected at least 2 memory chunks (first run + coalesced pending), got %d", len(all))
	}

	deadline = time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		svc.workerMu.Lock()
		_, exists := svc.workers[sessionID]
		svc.workerMu.Unlock()
		if !exists {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("worker state was not released")
}

func TestMemoryServiceBuildRecentHistory_UsesLimit(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 24; i++ {
		if _, err := addMessage(t, repo, sessionID, "user", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	history, err := svc.BuildRecentHistory(sessionID, constants.CompressedRecentHistoryLimit)
	if err != nil {
		t.Fatalf("build recent history failed: %v", err)
	}
	if len(history) != constants.CompressedRecentHistoryLimit {
		t.Fatalf("expected recent=%d, got %d", constants.CompressedRecentHistoryLimit, len(history))
	}
	contents := make(map[string]struct{}, len(history))
	for _, msg := range history {
		contents[msg.Content] = struct{}{}
	}
	if _, exists := contents["msg-0"]; exists {
		t.Fatalf("expected msg-0 trimmed, got history=%v", contents)
	}
	if _, exists := contents["msg-3"]; exists {
		t.Fatalf("expected msg-3 trimmed, got history=%v", contents)
	}
	if _, exists := contents["msg-23"]; !exists {
		t.Fatalf("expected latest message to remain, got history=%v", contents)
	}
}

func TestExternalSummaryUpsert_ProducesKeywords(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	if _, err := addMessage(t, repo, sessionID, "user", "璇疯浣忔垜鍋忓ソ golang 鍜?rag 瀹炵幇"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	summary := "用户偏好 golang，当前在做 rag 方案设计，关注 token 成本。"
	keywords := svc.TokenizeKeywords(summary)
	updated, err := repo.UpsertSessionMemoryIfNewer(domain.SessionMemoryUpsertInput{
		SessionID:          sessionID,
		Summary:            summary,
		Keywords:           keywords,
		SourceMessageCount: 1,
	})
	if err != nil {
		t.Fatalf("upsert summary failed: %v", err)
	}
	if !updated {
		t.Fatal("expected summary upsert updated=true")
	}

	item, err := repo.GetSessionMemory(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("get session memory failed: %v", err)
	}
	if item == nil {
		t.Fatal("expected session memory to exist")
	}
	if strings.TrimSpace(item.Summary) != summary {
		t.Fatalf("unexpected summary: %q", item.Summary)
	}
	if strings.TrimSpace(item.KeywordsText) == "" {
		t.Fatal("expected non-empty keywords_text")
	}
}

func TestFlattenMessages_IncludesTimestamp(t *testing.T) {
	at := time.Date(2026, 3, 18, 10, 0, 0, 0, time.Local)
	text := flattenMessages([]domain.Message{
		{Role: "user", Content: "hello", CreatedAt: at},
	})
	if !strings.Contains(text, "[") || !strings.Contains(text, "] user: hello") {
		t.Fatalf("unexpected flattened format: %q", text)
	}
	if !strings.Contains(text, at.Format(time.RFC3339)) {
		t.Fatalf("expected RFC3339 timestamp, got: %q", text)
	}
}

func TestMemoryServiceRetrieveMemories_UsesVectorStore(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)
	svc.SetEmbeddingService(mockEmbeddingService{
		vector: []float32{0.1, 0.2, 0.3},
	})
	svc.SetVectorStore(&mockVectorStore{
		hits: []domain.MemoryVectorSearchHit{
			{SessionID: "s2", Score: 0.95},
			{SessionID: "s1", Score: 0.90},
		},
	})

	if err := upsertSessionMemory(t, repo, domain.SessionMemoryUpsertInput{
		SessionID:          "s1",
		Summary:            "鐢ㄦ埛鍠滄 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 1,
	}); err != nil {
		t.Fatalf("upsert s1 failed: %v", err)
	}
	if err := upsertSessionMemory(t, repo, domain.SessionMemoryUpsertInput{
		SessionID:          "s2",
		Summary:            "鐢ㄦ埛鍏虫敞 rag",
		Keywords:           []string{"rag"},
		SourceMessageCount: 2,
	}); err != nil {
		t.Fatalf("upsert s2 failed: %v", err)
	}

	hits, err := svc.RetrieveMemories(context.Background(), "golang rag", "", 2)
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Memory.SessionID != "s2" {
		t.Fatalf("expected vector-ranked s2 first, got %s", hits[0].Memory.SessionID)
	}
}

func TestMemoryServiceRetrieveMemories_VectorErrorFallsBackToKeyword(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)
	svc.SetEmbeddingService(mockEmbeddingService{
		vector: []float32{0.1, 0.2, 0.3},
	})
	svc.SetVectorStore(&mockVectorStore{
		err: errors.New("vector unavailable"),
	})

	if err := upsertSessionMemory(t, repo, domain.SessionMemoryUpsertInput{
		SessionID:          "s1",
		Summary:            "鐢ㄦ埛鍠滄 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 1,
	}); err != nil {
		t.Fatalf("upsert s1 failed: %v", err)
	}

	hits, err := svc.RetrieveMemories(context.Background(), "golang", "", 1)
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected fallback keyword hit, got %d", len(hits))
	}
	if hits[0].Memory.SessionID != "s1" {
		t.Fatalf("expected keyword fallback s1, got %s", hits[0].Memory.SessionID)
	}
}

type mockEmbeddingService struct {
	vector []float32
	err    error
}

func (m mockEmbeddingService) Embed(_ context.Context, _ string) ([]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.vector, nil
}

func (m mockEmbeddingService) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([][]float32, 0, len(texts))
	for range texts {
		out = append(out, m.vector)
	}
	return out, nil
}

type mockVectorStore struct {
	hits        []domain.MemoryVectorSearchHit
	err         error
	upsertCalls int
}

func (m *mockVectorStore) UpsertSessionMemoryVector(_ context.Context, _ domain.MemoryVectorUpsertInput) error {
	m.upsertCalls++
	return m.err
}

func (m *mockVectorStore) SearchSimilarSessionIDs(_ context.Context, _ []float32, _ int, _ string) ([]domain.MemoryVectorSearchHit, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.hits, nil
}

func (m *mockVectorStore) SearchMemoriesInSession(_ context.Context, _ []float32, _ string, _ int) ([]domain.MemoryVectorSearchHit, error) {
	return nil, nil
}

func (m *mockVectorStore) DeleteMemoryVector(_ context.Context, _ string) error {
	return nil
}

func TestMemoryServiceRunSummaryOnce_UpsertsVectorWhenEnabled(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	if _, err := addMessage(t, repo, sessionID, "user", "璇疯浣忔垜鍠滄 golang"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}
	if _, err := addMessage(t, repo, sessionID, "assistant", "好的，我会记住"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	vectorStore := &mockVectorStore{}
	svc.SetEmbeddingService(mockEmbeddingService{
		vector: []float32{0.1, 0.2, 0.3},
	})
	svc.SetVectorStore(vectorStore)

	svc.UpdateSummaryAsync(sessionID, `{"ops":[{"action":"create","content":"鐢ㄦ埛鍠滄 golang"}]}`)
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if vectorStore.upsertCalls > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected vector upsert call after summary update")
}

func TestMemoryRetrievalSampleSet_VectorPathBeatsKeywordBaseline(t *testing.T) {
	repo := newTestRepo(t)

	memories := []domain.SessionMemoryUpsertInput{
		{SessionID: "m1", Summary: "用户学习 golang 并发", Keywords: []string{"golang", "并发"}, SourceMessageCount: 10},
		{SessionID: "m2", Summary: "用户正在做 RAG 架构", Keywords: []string{"rag", "检索"}, SourceMessageCount: 10},
		{SessionID: "m3", Summary: "用户关注成本优化", Keywords: []string{"成本", "优化"}, SourceMessageCount: 10},
	}
	for _, item := range memories {
		if err := upsertSessionMemory(t, repo, item); err != nil {
			t.Fatalf("seed memory failed: %v", err)
		}
	}

	keywordSvc := NewMemoryService(repo, nil)
	vectorSvc := NewMemoryService(repo, nil)
	vectorSvc.SetEmbeddingService(mockEmbeddingByQuery{
		vectors: map[string][]float32{
			"并发":  {1},
			"知识库": {2},
			"省钱":  {3},
		},
		defaultVec: []float32{9},
	})
	vectorSvc.SetVectorStore(&mockVectorStoreByQueryVec{
		rules: map[float32]string{
			1: "m1",
			2: "m2",
			3: "m3",
		},
	})

	cases := []struct {
		query  string
		expect string
	}{
		{query: "并发", expect: "m1"},
		{query: "知识库", expect: "m2"},
		{query: "省钱", expect: "m3"},
	}

	keywordTop1 := 0
	vectorTop1 := 0
	for _, tc := range cases {
		kHits, err := keywordSvc.RetrieveMemories(context.Background(), tc.query, "", 1)
		if err != nil {
			t.Fatalf("keyword retrieve failed: %v", err)
		}
		if len(kHits) > 0 && kHits[0].Memory.SessionID == tc.expect {
			keywordTop1++
		}

		vHits, err := vectorSvc.RetrieveMemories(context.Background(), tc.query, "", 1)
		if err != nil {
			t.Fatalf("vector retrieve failed: %v", err)
		}
		if len(vHits) > 0 && vHits[0].Memory.SessionID == tc.expect {
			vectorTop1++
		}
	}

	if vectorTop1 <= keywordTop1 {
		t.Fatalf("expected vector top1 better than keyword baseline, vector=%d keyword=%d", vectorTop1, keywordTop1)
	}
}

type mockEmbeddingByQuery struct {
	vectors    map[string][]float32
	defaultVec []float32
}

func (m mockEmbeddingByQuery) Embed(_ context.Context, text string) ([]float32, error) {
	trimmed := strings.TrimSpace(text)
	if vec, ok := m.vectors[trimmed]; ok {
		return vec, nil
	}
	return m.defaultVec, nil
}

func (m mockEmbeddingByQuery) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result = append(result, vec)
	}
	return result, nil
}

type mockVectorStoreByQueryVec struct {
	rules map[float32]string
}

func (m *mockVectorStoreByQueryVec) UpsertSessionMemoryVector(_ context.Context, _ domain.MemoryVectorUpsertInput) error {
	return nil
}

func (m *mockVectorStoreByQueryVec) SearchSimilarSessionIDs(_ context.Context, query []float32, _ int, _ string) ([]domain.MemoryVectorSearchHit, error) {
	if len(query) == 0 {
		return []domain.MemoryVectorSearchHit{}, nil
	}
	sessionID, ok := m.rules[query[0]]
	if !ok {
		return []domain.MemoryVectorSearchHit{}, nil
	}
	return []domain.MemoryVectorSearchHit{
		{SessionID: sessionID, Score: 0.99},
	}, nil
}

func (m *mockVectorStoreByQueryVec) SearchMemoriesInSession(_ context.Context, _ []float32, _ string, _ int) ([]domain.MemoryVectorSearchHit, error) {
	return nil, nil
}

func (m *mockVectorStoreByQueryVec) DeleteMemoryVector(_ context.Context, _ string) error {
	return nil
}

func addMessage(_ *testing.T, repo *repositories.Repository, sessionID, role, content string) (*domain.Message, error) {
	return repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	})
}

func upsertSessionMemory(_ *testing.T, repo *repositories.Repository, input domain.SessionMemoryUpsertInput) error {
	_, err := repo.UpsertSessionMemoryIfNewer(input)
	return err
}
func newTestRepo(t *testing.T) *repositories.Repository {
	t.Helper()
	return repositories.New(repositories.NewSQLiteDBTest(t, "services_test"))
}
