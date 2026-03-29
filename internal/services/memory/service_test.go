package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/repositories"
)

func TestParseTurnMemoryPayload_RejectsInvalidPayload(t *testing.T) {
	if _, err := parseTurnMemoryPayload("not-json"); err == nil {
		t.Fatal("expected parser error")
	}
}

func TestParseTurnMemoryPayload_AcceptsNonStringStickyValue(t *testing.T) {
	payload, err := parseTurnMemoryPayload(`{"turn_summary":"s","sticky":[{"kind":"task","key":"budget","value":500,"summary":"预算 500","confidence":0.9,"action":"upsert"}]}`)
	if err != nil {
		t.Fatalf("parse payload failed: %v", err)
	}
	if len(payload.Sticky) != 1 {
		t.Fatalf("expected 1 sticky item, got %d", len(payload.Sticky))
	}
	if payload.Sticky[0].Value != "500" {
		t.Fatalf("expected sticky value to normalize to string, got %q", payload.Sticky[0].Value)
	}
}

func TestMemoryServiceEnqueueTurnMemory_PersistsEpisodeAndSticky(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	if _, err := addMessage(t, repo, session.ID, "user", "之后都用中文回复"); err != nil {
		t.Fatalf("add user message failed: %v", err)
	}
	assistant, err := addMessage(t, repo, session.ID, "assistant", "好的")
	if err != nil {
		t.Fatalf("add assistant message failed: %v", err)
	}

	svc.EnqueueTurnMemory(session.ID, assistant.ID, `{"turn_summary":"用户要求后续都用中文回复","topic_hint":"回复偏好","keywords":["中文","回复"],"sticky":[{"kind":"preference","key":"reply_language","value":"zh-cn","summary":"默认中文回复","confidence":0.96,"action":"upsert"}]}`)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		episode, getErr := repo.GetOpenEpisodeMemory(context.Background(), session.ID)
		if getErr != nil {
			t.Fatalf("get open episode failed: %v", getErr)
		}
		sticky, listErr := repo.ListStickyMemoriesForPrompt(context.Background(), session.ID, 10, time.Now())
		if listErr != nil {
			t.Fatalf("list sticky failed: %v", listErr)
		}
		if episode != nil && len(sticky) == 1 {
			if episode.TopicKey != "回复偏好" {
				t.Fatalf("unexpected topic key: %s", episode.TopicKey)
			}
			if sticky[0].Key != "reply_language" || sticky[0].Value != "zh-cn" {
				t.Fatalf("unexpected sticky memory: %#v", sticky[0])
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected memory persisted")
}

func TestMemoryServiceEnqueueTurnMemory_SplitsTopicsAndReopensRecentTopic(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	session, err := repo.CreateSession(context.Background(), "s")
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	firstAssistant := seedTurn(t, repo, session.ID, "帮我规划日本旅游", "先看日本签证")
	svc.EnqueueTurnMemory(session.ID, firstAssistant.ID, `{"turn_summary":"用户在聊日本旅游和签证","topic_hint":"日本旅游","keywords":["日本","旅游","签证"],"sticky":[]}`)
	waitOpenTopic(t, repo, session.ID, "日本旅游")

	secondAssistant := seedTurn(t, repo, session.ID, "那酿豆腐怎么做", "先准备豆腐和肉馅")
	svc.EnqueueTurnMemory(session.ID, secondAssistant.ID, `{"turn_summary":"用户改聊酿豆腐做法","topic_hint":"酿豆腐","keywords":["酿豆腐","豆腐","做菜"],"sticky":[]}`)
	waitOpenTopic(t, repo, session.ID, "酿豆腐")

	closedTravel, err := repo.GetLatestClosedEpisodeByTopicKey(context.Background(), session.ID, "日本旅游")
	if err != nil {
		t.Fatalf("get closed travel episode failed: %v", err)
	}
	if closedTravel == nil {
		t.Fatal("expected closed travel episode")
	}

	thirdAssistant := seedTurn(t, repo, session.ID, "继续说日本旅游", "我们继续看行程")
	svc.EnqueueTurnMemory(session.ID, thirdAssistant.ID, `{"turn_summary":"用户回到日本旅游行程","topic_hint":"日本旅游","keywords":["日本","旅游","行程"],"sticky":[]}`)
	waitOpenTopic(t, repo, session.ID, "日本旅游")

	reopened, err := repo.GetOpenEpisodeMemory(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get reopened episode failed: %v", err)
	}
	if reopened == nil || reopened.ID != closedTravel.ID {
		t.Fatalf("expected recent topic reopen, got %#v want %s", reopened, closedTravel.ID)
	}
}

func TestMemoryServiceBuildMemoryContext_InjectsStickyAndRelevantEpisodes(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	if _, err := repo.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
		SessionID:      "s1",
		Kind:           domain.StickyMemoryKindPreference,
		Key:            "reply_language",
		Value:          "zh-cn",
		Summary:        "默认中文回复",
		Confidence:     0.95,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		LastSeenAt:     time.Now(),
	}); err != nil {
		t.Fatalf("seed sticky failed: %v", err)
	}
	if _, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "日本旅游",
		Title:          "日本旅游",
		Summary:        "用户之前聊过日本签证和旅行安排",
		Keywords:       []string{"日本", "旅游", "签证"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   4,
		TurnCount:      2,
		LastActiveAt:   time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed episode failed: %v", err)
	}

	history := []domain.Message{
		{SessionID: "s1", Role: "user", Content: "帮我继续看日本签证", Seq: 9, CreatedAt: time.Now()},
		{SessionID: "s1", Role: "assistant", Content: "可以，我们继续", Seq: 10, CreatedAt: time.Now()},
	}
	ctxText := svc.BuildMemoryContext(context.Background(), "s1", history)
	if !strings.Contains(ctxText, "<sticky_memories>") {
		t.Fatalf("expected sticky memories section, got %q", ctxText)
	}
	if !strings.Contains(ctxText, "<episode_memories>") {
		t.Fatalf("expected episode memories section, got %q", ctxText)
	}
	if !strings.Contains(ctxText, "日本旅游") {
		t.Fatalf("expected episode title in context, got %q", ctxText)
	}
}

func TestMemoryServiceQueryForAgent_FallsBackToKeywordSearch(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	if _, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "日本旅游",
		Title:          "日本旅游",
		Summary:        "用户之前聊过日本签证和旅行安排",
		Keywords:       []string{"日本", "旅游", "签证"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   4,
		TurnCount:      2,
		LastActiveAt:   time.Now(),
	}); err != nil {
		t.Fatalf("seed episode failed: %v", err)
	}

	result, err := svc.QueryForAgent(context.Background(), "s1", "继续日本签证", 3)
	if err != nil {
		t.Fatalf("query for agent failed: %v", err)
	}
	if !strings.Contains(result.Output, "<memory_query_result>") {
		t.Fatalf("expected memory query output, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "episode") {
		t.Fatalf("expected episode hit in output, got %q", result.Output)
	}
}

func TestMemoryServiceQueryForAgent_SearchesAcrossSessions(t *testing.T) {
	repo := newTestRepo(t)
	svc := NewMemoryService(repo, nil)

	if _, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s2",
		TopicKey:       "golang",
		Title:          "Golang",
		Summary:        "记录了 Go 并发和 channel 的经验",
		Keywords:       []string{"golang", "go", "channel"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		TurnCount:      1,
		LastActiveAt:   time.Now(),
	}); err != nil {
		t.Fatalf("seed cross-session episode failed: %v", err)
	}
	if _, err := repo.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
		SessionID:      "s3",
		Kind:           domain.StickyMemoryKindPreference,
		Key:            "editor_theme",
		Value:          "golang-dark",
		Summary:        "偏好 golang-dark 主题",
		Confidence:     0.92,
		SourceStartSeq: 1,
		SourceEndSeq:   1,
		LastSeenAt:     time.Now(),
	}); err != nil {
		t.Fatalf("seed cross-session sticky failed: %v", err)
	}

	result, err := svc.QueryForAgent(context.Background(), "s1", "golang", 5)
	if err != nil {
		t.Fatalf("query for agent failed: %v", err)
	}
	if !strings.Contains(result.Output, "Golang") {
		t.Fatalf("expected cross-session episode hit, got %q", result.Output)
	}
	if !strings.Contains(result.Output, "editor_theme") {
		t.Fatalf("expected cross-session sticky hit, got %q", result.Output)
	}
}

func seedTurn(t *testing.T, repo *repositories.Repository, sessionID, userContent, assistantContent string) *domain.Message {
	t.Helper()
	if _, err := addMessage(t, repo, sessionID, "user", userContent); err != nil {
		t.Fatalf("add user message failed: %v", err)
	}
	assistant, err := addMessage(t, repo, sessionID, "assistant", assistantContent)
	if err != nil {
		t.Fatalf("add assistant message failed: %v", err)
	}
	return assistant
}

func waitOpenTopic(t *testing.T, repo *repositories.Repository, sessionID, topicKey string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		item, err := repo.GetOpenEpisodeMemory(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("get open episode failed: %v", err)
		}
		if item != nil && item.TopicKey == topicKey {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected open topic %s", topicKey)
}

func addMessage(_ *testing.T, repo *repositories.Repository, sessionID, role, content string) (*domain.Message, error) {
	return repo.AddMessageWithInput(context.Background(), domain.AddMessageInput{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	})
}

func newTestRepo(t *testing.T) *repositories.Repository {
	t.Helper()
	return repositories.New(repositories.NewSQLiteDBTest(t, "services_test"))
}
