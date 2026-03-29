package domain

import (
	"context"
	"time"
)

// MemoryStore 记忆服务持久化接口。
type MemoryStore interface {
	GetMessageByID(ctx context.Context, id string) (*Message, error)
	ListRecentSessionMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)

	CreateEpisodeMemory(input EpisodeMemoryCreateInput) (*EpisodeMemory, error)
	UpdateEpisodeMemory(input EpisodeMemoryUpdateInput) error
	GetOpenEpisodeMemory(ctx context.Context, sessionID string) (*EpisodeMemory, error)
	GetLatestClosedEpisodeByTopicKey(ctx context.Context, sessionID, topicKey string) (*EpisodeMemory, error)
	GetEpisodeMemoriesByIDs(ids []string) ([]EpisodeMemory, error)
	SearchEpisodeMemories(ctx context.Context, input EpisodeMemorySearchInput) ([]EpisodeMemorySearchHit, error)

	UpsertStickyMemory(input StickyMemoryUpsertInput) (*StickyMemory, error)
	DeleteStickyMemory(ctx context.Context, sessionID, kind, key string) error
	ListStickyMemoriesForPrompt(ctx context.Context, sessionID string, limit int, now time.Time) ([]StickyMemory, error)
	SearchStickyMemories(ctx context.Context, sessionID, query string, limit int, now time.Time) ([]StickyMemorySearchHit, error)
}
