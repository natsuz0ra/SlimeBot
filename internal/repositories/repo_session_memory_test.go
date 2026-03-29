package repositories

import (
	"context"
	"testing"
	"time"

	"slimebot/internal/domain"
)

func TestEpisodeMemoryRepository_CreateUpdateAndSearch(t *testing.T) {
	repo := newSessionMemoryRepo(t)
	now := time.Now()

	first, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "travel",
		Title:          "旅游规划",
		Summary:        "用户在聊日本旅行安排",
		Keywords:       []string{"旅游", "日本", "签证"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   4,
		TurnCount:      2,
		LastActiveAt:   now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create episode failed: %v", err)
	}

	if _, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "recipe",
		Title:          "酿豆腐",
		Summary:        "用户在聊酿豆腐做法",
		Keywords:       []string{"酿豆腐", "豆腐", "做菜"},
		State:          domain.EpisodeMemoryStateOpen,
		SourceStartSeq: 5,
		SourceEndSeq:   8,
		TurnCount:      2,
		LastActiveAt:   now,
	}); err != nil {
		t.Fatalf("create second episode failed: %v", err)
	}

	if err := repo.UpdateEpisodeMemory(domain.EpisodeMemoryUpdateInput{
		ID:             first.ID,
		SessionID:      "s1",
		Title:          "旅游计划",
		Summary:        "用户在聊日本旅游计划和签证",
		Keywords:       []string{"旅游", "日本", "签证", "行程"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   6,
		TurnCount:      3,
		LastActiveAt:   now,
	}); err != nil {
		t.Fatalf("update episode failed: %v", err)
	}

	hits, err := repo.SearchEpisodeMemories(context.Background(), domain.EpisodeMemorySearchInput{
		SessionID:       "s1",
		Query:           "日本 签证 旅游",
		Limit:           5,
		ExcludeStartSeq: 7,
		ExcludeEndSeq:   12,
		Now:             now,
	})
	if err != nil {
		t.Fatalf("search episodes failed: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %d", len(hits))
	}
	if hits[0].Episode.ID != first.ID {
		t.Fatalf("unexpected first hit: %s", hits[0].Episode.ID)
	}
}

func TestEpisodeMemoryRepository_SearchEpisodeMemories_GlobalWhenSessionEmpty(t *testing.T) {
	repo := newSessionMemoryRepo(t)
	now := time.Now()

	first, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "golang",
		Title:          "Golang",
		Summary:        "Go 并发经验",
		Keywords:       []string{"golang", "go"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		TurnCount:      1,
		LastActiveAt:   now,
	})
	if err != nil {
		t.Fatalf("create first episode failed: %v", err)
	}
	second, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s2",
		TopicKey:       "python",
		Title:          "Python",
		Summary:        "Python 脚本经验",
		Keywords:       []string{"python"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		TurnCount:      1,
		LastActiveAt:   now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create second episode failed: %v", err)
	}

	hits, err := repo.SearchEpisodeMemories(context.Background(), domain.EpisodeMemorySearchInput{
		Query: "golang python",
		Limit: 5,
		Now:   now,
	})
	if err != nil {
		t.Fatalf("search episodes failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected two hits, got %d", len(hits))
	}
	got := map[string]bool{
		hits[0].Episode.ID: true,
		hits[1].Episode.ID: true,
	}
	if !got[first.ID] || !got[second.ID] {
		t.Fatalf("unexpected hit ids: %#v", hits)
	}
}

func TestEpisodeMemoryRepository_OpenEpisodeAndTopicLookup(t *testing.T) {
	repo := newSessionMemoryRepo(t)
	now := time.Now()

	closed, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "travel",
		Title:          "旅游",
		Summary:        "第一段旅游",
		Keywords:       []string{"旅游"},
		State:          domain.EpisodeMemoryStateClosed,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		TurnCount:      1,
		LastActiveAt:   now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("create closed episode failed: %v", err)
	}

	open, err := repo.CreateEpisodeMemory(domain.EpisodeMemoryCreateInput{
		SessionID:      "s1",
		TopicKey:       "recipe",
		Title:          "做菜",
		Summary:        "当前在聊做菜",
		Keywords:       []string{"做菜"},
		State:          domain.EpisodeMemoryStateOpen,
		SourceStartSeq: 3,
		SourceEndSeq:   4,
		TurnCount:      1,
		LastActiveAt:   now,
	})
	if err != nil {
		t.Fatalf("create open episode failed: %v", err)
	}

	gotOpen, err := repo.GetOpenEpisodeMemory(context.Background(), "s1")
	if err != nil {
		t.Fatalf("get open episode failed: %v", err)
	}
	if gotOpen == nil || gotOpen.ID != open.ID {
		t.Fatalf("unexpected open episode: %#v", gotOpen)
	}

	gotClosed, err := repo.GetLatestClosedEpisodeByTopicKey(context.Background(), "s1", "travel")
	if err != nil {
		t.Fatalf("get closed episode failed: %v", err)
	}
	if gotClosed == nil || gotClosed.ID != closed.ID {
		t.Fatalf("unexpected closed episode: %#v", gotClosed)
	}
}

func TestStickyMemoryRepository_UpsertAndDelete(t *testing.T) {
	repo := newSessionMemoryRepo(t)
	now := time.Now()

	first, err := repo.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
		SessionID:      "s1",
		Kind:           domain.StickyMemoryKindPreference,
		Key:            "reply_language",
		Value:          "zh-cn",
		Summary:        "默认中文回复",
		Confidence:     0.91,
		SourceStartSeq: 1,
		SourceEndSeq:   2,
		LastSeenAt:     now,
	})
	if err != nil {
		t.Fatalf("upsert sticky failed: %v", err)
	}

	second, err := repo.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
		SessionID:      "s1",
		Kind:           domain.StickyMemoryKindPreference,
		Key:            "reply_language",
		Value:          "zh-cn",
		Summary:        "用户持续要求中文回复",
		Confidence:     0.97,
		SourceStartSeq: 1,
		SourceEndSeq:   4,
		LastSeenAt:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("second upsert sticky failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same sticky id, got %s and %s", first.ID, second.ID)
	}

	items, err := repo.ListStickyMemoriesForPrompt(context.Background(), "s1", 10, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("list sticky failed: %v", err)
	}
	if len(items) != 1 || items[0].Summary != "用户持续要求中文回复" {
		t.Fatalf("unexpected sticky items: %#v", items)
	}

	if err := repo.DeleteStickyMemory(context.Background(), "s1", domain.StickyMemoryKindPreference, "reply_language"); err != nil {
		t.Fatalf("delete sticky failed: %v", err)
	}

	items, err = repo.ListStickyMemoriesForPrompt(context.Background(), "s1", 10, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("list sticky after delete failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected sticky deleted, got %#v", items)
	}
}

func TestStickyMemoryRepository_SearchStickyMemories_GlobalWhenSessionEmpty(t *testing.T) {
	repo := newSessionMemoryRepo(t)
	now := time.Now()

	if _, err := repo.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
		SessionID:      "s1",
		Kind:           domain.StickyMemoryKindPreference,
		Key:            "editor_theme",
		Value:          "golang-dark",
		Summary:        "偏好 golang-dark 主题",
		Confidence:     0.91,
		SourceStartSeq: 1,
		SourceEndSeq:   1,
		LastSeenAt:     now,
	}); err != nil {
		t.Fatalf("upsert first sticky failed: %v", err)
	}
	if _, err := repo.UpsertStickyMemory(domain.StickyMemoryUpsertInput{
		SessionID:      "s2",
		Kind:           domain.StickyMemoryKindPreference,
		Key:            "language",
		Value:          "python",
		Summary:        "偏好 python 示例",
		Confidence:     0.88,
		SourceStartSeq: 1,
		SourceEndSeq:   1,
		LastSeenAt:     now,
	}); err != nil {
		t.Fatalf("upsert second sticky failed: %v", err)
	}

	hits, err := repo.SearchStickyMemories(context.Background(), "", "golang python", 5, now)
	if err != nil {
		t.Fatalf("search sticky failed: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected two hits, got %d", len(hits))
	}
}

func newSessionMemoryRepo(t *testing.T) *Repository {
	t.Helper()
	return New(NewSQLiteDBTest(t, "memory_repo"))
}
