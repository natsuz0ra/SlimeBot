package domain

import "context"

// MemoryStore 记忆服务持久化接口。
type MemoryStore interface {
	CountSessionMessages(sessionID string) (int64, error)
	UpsertSessionMemoryIfNewer(input SessionMemoryUpsertInput) (bool, error)
	SearchMemoriesByKeywords(keywords []string, limit int, excludeSessionID string) ([]SessionMemorySearchHit, error)
	ListRecentSessionMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	GetSessionMemory(ctx context.Context, sessionID string) (*SessionMemory, error)
	GetSessionMemoriesBySessionIDs(sessionIDs []string) ([]SessionMemory, error)
	GetSessionMemoriesByIDs(ids []string) ([]SessionMemory, error)
	CountActiveSessionMemories(sessionID string) (int64, error)
	ListActiveSessionMemories(ctx context.Context, sessionID string) ([]SessionMemory, error)
	ListRecentActiveSessionMemories(sessionID string, limit int) ([]SessionMemory, error)
	CreateSessionMemory(input SessionMemoryCreateInput) (*SessionMemory, error)
	UpdateSessionMemoryContent(id, sessionID, summary string, keywords []string, sourceMessageCount int) error
	SoftDeleteSessionMemory(id, sessionID string) error
}
