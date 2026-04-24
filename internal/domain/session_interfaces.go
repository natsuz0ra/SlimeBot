package domain

import (
	"context"
	"time"
)

// SessionStore persists sessions: CRUD, message pagination, and tool-call records.
type SessionStore interface {
	ListSessions(limit int, offset int, query string) ([]Session, error)
	CreateSession(ctx context.Context, name string) (*Session, error)
	RenameSessionByUser(id, name string) error
	DeleteSession(id string) error
	ListSessionMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]Message, bool, error)
	ListSessionToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]ToolCallRecord, error)
	ListSessionThinkingRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]ThinkingRecord, error)
}
