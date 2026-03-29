package domain

import (
	"context"
	"time"
)

const (
	EpisodeMemoryStateOpen     = "open"
	EpisodeMemoryStateClosed   = "closed"
	EpisodeMemoryStateArchived = "archived"

	StickyMemoryKindPreference = "preference"
	StickyMemoryKindConstraint = "constraint"
	StickyMemoryKindTask       = "task"

	StickyMemoryStatusActive   = "active"
	StickyMemoryStatusDeleted  = "deleted"
	StickyMemoryStatusArchived = "archived"
)

type EpisodeMemoryCreateInput struct {
	SessionID      string
	TopicKey       string
	Title          string
	Summary        string
	Keywords       []string
	State          string
	SourceStartSeq int64
	SourceEndSeq   int64
	TurnCount      int
	LastActiveAt   time.Time
}

type EpisodeMemoryUpdateInput struct {
	ID             string
	SessionID      string
	TopicKey       string
	Title          string
	Summary        string
	Keywords       []string
	State          string
	SourceStartSeq int64
	SourceEndSeq   int64
	TurnCount      int
	LastActiveAt   time.Time
}

type EpisodeMemorySearchInput struct {
	SessionID       string
	Query           string
	Limit           int
	ExcludeStartSeq int64
	ExcludeEndSeq   int64
	Now             time.Time
}

type EpisodeMemorySearchHit struct {
	Episode         EpisodeMemory
	MatchedKeywords []string
	Score           float64
}

type StickyMemoryUpsertInput struct {
	SessionID      string
	Kind           string
	Key            string
	Value          string
	Summary        string
	Confidence     float64
	SourceStartSeq int64
	SourceEndSeq   int64
	LastSeenAt     time.Time
	ExpiresAt      *time.Time
}

type StickyMemorySearchHit struct {
	Memory          StickyMemory
	MatchedKeywords []string
	Score           float64
}

type AddMessageInput struct {
	SessionID         string
	Role              string
	Content           string
	IsInterrupted     bool
	IsStopPlaceholder bool
	Attachments       []MessageAttachment
}

type ToolCallStartRecordInput struct {
	SessionID        string
	RequestID        string
	ToolCallID       string
	ToolName         string
	Command          string
	Params           map[string]string
	Status           string
	RequiresApproval bool
	StartedAt        time.Time
}

type ToolCallResultRecordInput struct {
	SessionID  string
	RequestID  string
	ToolCallID string
	Status     string
	Output     string
	Error      string
	FinishedAt time.Time
}

type MemoryVectorStore interface {
	UpsertSessionMemoryVector(ctx context.Context, input MemoryVectorUpsertInput) error
	SearchMemoriesInSession(ctx context.Context, queryVector []float32, sessionID string, limit int) ([]MemoryVectorSearchHit, error)
}

type MemoryVectorUpsertInput struct {
	MemoryID  string
	SessionID string
	Vector    []float32
	Payload   map[string]any
}

type MemoryVectorSearchHit struct {
	SessionID string
	MemoryID  string
	Score     float64
}
