package domain

import (
	"context"
	"time"
)

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
	ParentToolCallID string
	SubagentRunID    string
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

type ThinkingStartRecordInput struct {
	SessionID  string
	RequestID  string
	ThinkingID string
	StartedAt  time.Time
}

type ThinkingChunkRecordInput struct {
	SessionID  string
	RequestID  string
	ThinkingID string
	Chunk      string
}

type ThinkingFinishRecordInput struct {
	SessionID  string
	RequestID  string
	ThinkingID string
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
