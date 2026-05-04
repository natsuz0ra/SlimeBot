package domain

import (
	llmsvc "slimebot/internal/services/llm"
	"time"
)

type AddMessageInput struct {
	SessionID         string
	Role              string
	Content           string
	IsInterrupted     bool
	IsStopPlaceholder bool
	Attachments       []MessageAttachment
	TokenUsage        *llmsvc.TokenUsage
	CreatedAt         time.Time
}

type ToolCallStartRecordInput struct {
	SessionID        string
	RequestID        string
	ToolCallID       string
	ToolName         string
	Command          string
	Params           map[string]any
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
	Metadata   any
	FinishedAt time.Time
}

type ThinkingStartRecordInput struct {
	SessionID        string
	RequestID        string
	ThinkingID       string
	ParentToolCallID string
	SubagentRunID    string
	StartedAt        time.Time
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
