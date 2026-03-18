package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
	"slimebot/backend/internal/testutil"
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
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < consts.CompressHistoryThreshold; i++ {
		if _, err := repo.AddMessage(sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	ok, count, err := svc.ShouldCompressContext(sessionID)
	if err != nil {
		t.Fatalf("should compress failed: %v", err)
	}
	if !ok {
		t.Fatal("expected compress=true when threshold reached")
	}
	if count != consts.CompressHistoryThreshold {
		t.Fatalf("unexpected count: %d", count)
	}
}

func TestMemoryServiceRetrieveMemoriesRanking(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          "s1",
		Summary:            "用户喜欢 golang 与 rag，关注 token 成本",
		Keywords:           []string{"golang", "rag", "token"},
		SourceMessageCount: 20,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          "s2",
		Summary:            "用户只提到了 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 10,
	}); err != nil {
		t.Fatalf("upsert memory failed: %v", err)
	}

	hits, err := svc.RetrieveMemories([]string{"golang", "rag"}, "", 5)
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

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 3; i++ {
		if _, err := repo.AddMessage(sessionID, "user", "hello"); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	start := time.Now()
	svc.UpdateSummaryAsync(ModelRuntimeConfig{BaseURL: "http://invalid", APIKey: "x", Model: "y"}, sessionID)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected async call to return quickly, elapsed=%s", elapsed)
	}
}

func TestMemoryServiceChatOnceWithRetry_TimeoutThenSuccess(t *testing.T) {
	svc := NewMemoryService(nil, nil)
	callCount := int32(0)
	svc.chatInvoker = func(_ context.Context, _ ModelRuntimeConfig, _ []ChatMessage) (string, error) {
		attempt := atomic.AddInt32(&callCount, 1)
		if attempt == 1 {
			return "", context.DeadlineExceeded
		}
		return "{\"need_memory\":true,\"keywords\":[\"slimebot\"],\"reason\":\"ok\"}", nil
	}

	reply, attempts, _, err := svc.chatOnceWithRetry(
		context.Background(),
		ModelRuntimeConfig{},
		[]ChatMessage{{Role: "user", Content: "test"}},
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

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 4; i++ {
		if _, err := repo.AddMessage(sessionID, "user", fmt.Sprintf("hello-%d", i)); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	firstStarted := make(chan struct{})
	allowFirst := make(chan struct{})
	var active int32
	var maxActive int32
	var calls int32

	svc.chatInvoker = func(_ context.Context, _ ModelRuntimeConfig, _ []ChatMessage) (string, error) {
		n := atomic.AddInt32(&calls, 1)
		current := atomic.AddInt32(&active, 1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}
		defer atomic.AddInt32(&active, -1)

		if n == 1 {
			close(firstStarted)
			<-allowFirst
		}
		return fmt.Sprintf("summary-%d", n), nil
	}

	svc.UpdateSummaryAsync(ModelRuntimeConfig{}, sessionID)
	<-firstStarted
	svc.UpdateSummaryAsync(ModelRuntimeConfig{}, sessionID)
	svc.UpdateSummaryAsync(ModelRuntimeConfig{}, sessionID)
	close(allowFirst)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected exactly 2 merged runs, got %d", atomic.LoadInt32(&calls))
	}
	if atomic.LoadInt32(&maxActive) > 1 {
		t.Fatalf("expected serialized worker, max concurrent=%d", atomic.LoadInt32(&maxActive))
	}

	deadline = time.Now().Add(2 * time.Second)
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

func TestChatServiceBuildContextMessages_NoDuplicateUserMessage(t *testing.T) {
	repo := newTestRepo(t)
	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	if _, err := repo.AddMessage(sessionID, "user", "only-once"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	svc := &ChatService{repo: repo}
	msgs, err := svc.BuildContextMessages(context.Background(), sessionID, ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("build context failed: %v", err)
	}

	userCount := 0
	for _, msg := range msgs {
		if msg.Role == "user" && strings.TrimSpace(msg.Content) == "only-once" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("expected single user message, got %d", userCount)
	}
}

func TestMemoryServiceBuildRecentHistory_UsesLimit(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 24; i++ {
		if _, err := repo.AddMessage(sessionID, "user", fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	history, err := svc.BuildRecentHistory(sessionID, consts.CompressedRecentHistoryLimit)
	if err != nil {
		t.Fatalf("build recent history failed: %v", err)
	}
	if len(history) != consts.CompressedRecentHistoryLimit {
		t.Fatalf("expected recent=%d, got %d", consts.CompressedRecentHistoryLimit, len(history))
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

func TestChatServiceBuildContextMessages_UsesMemoryAndRecentHistory(t *testing.T) {
	repo := newTestRepo(t)
	memorySvc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < 12; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		if _, err := repo.AddMessage(sessionID, role, fmt.Sprintf("turn-%d", i)); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
		time.Sleep(1 * time.Millisecond)
	}

	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          sessionID,
		Summary:            "当前会话关注 golang 性能优化",
		Keywords:           []string{"golang", "性能"},
		SourceMessageCount: 12,
	}); err != nil {
		t.Fatalf("upsert current session memory failed: %v", err)
	}
	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          "other",
		Summary:            "历史会话也提到 golang",
		Keywords:           []string{"golang"},
		SourceMessageCount: 20,
	}); err != nil {
		t.Fatalf("upsert other session memory failed: %v", err)
	}

	svc := &ChatService{repo: repo, memory: memorySvc}
	msgs, err := svc.BuildContextMessages(context.Background(), sessionID, ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("build context failed: %v", err)
	}

	if len(msgs) != 15 {
		t.Fatalf("expected 15 messages (system + memory + protocol + recent12), got %d", len(msgs))
	}
	if msgs[1].Role != "developer" || !strings.Contains(msgs[1].Content, "<memory_context>") {
		t.Fatalf("expected memory context developer message, got role=%q", msgs[1].Role)
	}
	if msgs[2].Role != "developer" || !strings.Contains(msgs[2].Content, "Final response protocol (strict):") {
		t.Fatalf("expected protocol developer message, got role=%q", msgs[2].Role)
	}
	if !strings.Contains(msgs[1].Content, "<current_session_summary>") {
		t.Fatalf("expected current session summary in memory context, got=%q", msgs[1].Content)
	}
	if strings.Contains(msgs[1].Content, "<retrieved_memories>") {
		t.Fatalf("did not expect cross-session memories in context, got=%q", msgs[1].Content)
	}
	if strings.Contains(msgs[1].Content, "历史会话也提到 golang") {
		t.Fatalf("did not expect other-session summary in context, got=%q", msgs[1].Content)
	}

	historyPart := msgs[3:]
	if len(historyPart) != 12 {
		t.Fatalf("expected %d history messages, got %d", 12, len(historyPart))
	}
	for i, msg := range historyPart {
		want := fmt.Sprintf("turn-%d", i)
		if msg.Content != want {
			t.Fatalf("unexpected history at index %d: got=%q want=%q", i, msg.Content, want)
		}
	}
}

func TestChatServiceBuildContextMessages_UnderThresholdUsesMemoryAndHistory(t *testing.T) {
	repo := newTestRepo(t)
	memorySvc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	for i := 0; i < consts.CompressHistoryThreshold-1; i++ {
		if _, err := repo.AddMessage(sessionID, "user", fmt.Sprintf("short-%d", i)); err != nil {
			t.Fatalf("add message failed: %v", err)
		}
	}

	if err := repo.UpsertSessionMemory(repositories.SessionMemoryUpsertInput{
		SessionID:          sessionID,
		Summary:            "已有摘要但不应在阈值以下注入",
		Keywords:           []string{"摘要"},
		SourceMessageCount: consts.CompressHistoryThreshold - 1,
	}); err != nil {
		t.Fatalf("upsert session memory failed: %v", err)
	}

	svc := &ChatService{repo: repo, memory: memorySvc}
	msgs, err := svc.BuildContextMessages(context.Background(), sessionID, ModelRuntimeConfig{})
	if err != nil {
		t.Fatalf("build context failed: %v", err)
	}

	expected := 3 + (consts.CompressHistoryThreshold - 1)
	if len(msgs) != expected {
		t.Fatalf("expected %d messages (system + memory + protocol + full history), got %d", expected, len(msgs))
	}
	if msgs[1].Role != "developer" || !strings.Contains(msgs[1].Content, "<memory_context>") {
		t.Fatalf("expected memory context developer message, got role=%q", msgs[1].Role)
	}
	if msgs[2].Role != "developer" || !strings.Contains(msgs[2].Content, "Final response protocol (strict):") {
		t.Fatalf("expected protocol developer message, got role=%q", msgs[2].Role)
	}
}

func TestExternalSummaryUpsert_ProducesKeywords(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession("s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	sessionID := session.ID
	if _, err := repo.AddMessage(sessionID, "user", "请记住我偏好 golang 和 rag 实现"); err != nil {
		t.Fatalf("add message failed: %v", err)
	}

	summary := "用户偏好 golang，当前在做 rag 方案设计，关注 token 成本。"
	keywords := svc.TokenizeKeywords(summary)
	updated, err := repo.UpsertSessionMemoryIfNewer(repositories.SessionMemoryUpsertInput{
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

	item, err := repo.GetSessionMemory(sessionID)
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
	text := flattenMessages([]models.Message{
		{Role: "user", Content: "hello", CreatedAt: at},
	})
	if !strings.Contains(text, "[") || !strings.Contains(text, "] user: hello") {
		t.Fatalf("unexpected flattened format: %q", text)
	}
	if !strings.Contains(text, at.Format(time.RFC3339)) {
		t.Fatalf("expected RFC3339 timestamp, got: %q", text)
	}
}

func newTestRepo(t *testing.T) *repositories.Repository {
	t.Helper()
	return repositories.New(testutil.NewSQLiteDB(t, "services_test"))
}
