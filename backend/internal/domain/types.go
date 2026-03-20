package domain

import (
	"context"
	"time"
)

type SessionMemoryUpsertInput struct {
	SessionID          string
	Summary            string
	Keywords           []string
	SourceMessageCount int
}

type SessionMemorySearchHit struct {
	Memory          SessionMemory
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
	SearchSimilarSessionIDs(ctx context.Context, queryVector []float32, limit int, excludeSessionID string) ([]MemoryVectorSearchHit, error)
}

type MemoryVectorUpsertInput struct {
	SessionID string
	Vector    []float32
	Payload   map[string]any
}

type MemoryVectorSearchHit struct {
	SessionID string
	Score     float64
}
