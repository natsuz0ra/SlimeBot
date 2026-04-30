package domain

import (
	"context"
	"time"
)

// SessionStore persists sessions: CRUD, message pagination, and tool-call records.
type SessionStore interface {
	ListSessions(ctx context.Context, limit int, offset int, query string) ([]Session, error)
	CreateSession(ctx context.Context, name string) (*Session, error)
	RenameSessionByUser(ctx context.Context, id, name string) error
	DeleteSession(ctx context.Context, id string) error
	ListSessionMessagesPage(ctx context.Context, sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]Message, bool, error)
	ListSessionToolCallRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]ToolCallRecord, error)
	ListSessionThinkingRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]ThinkingRecord, error)
}
