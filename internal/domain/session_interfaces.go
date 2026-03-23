package domain

import (
	"context"
	"time"
)

// SessionStore 会话存储接口：CRUD、消息分页和工具调用记录查询。
type SessionStore interface {
	ListSessions(limit int, offset int, query string) ([]Session, error)
	CreateSession(ctx context.Context, name string) (*Session, error)
	RenameSessionByUser(id, name string) error
	DeleteSession(id string) error
	ListSessionMessages(sessionID string) ([]Message, error)
	ListSessionMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]Message, bool, error)
	ListSessionToolCallRecords(sessionID string) ([]ToolCallRecord, error)
	ListSessionToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]ToolCallRecord, error)
	SetSessionModel(sessionID, modelConfigID string) error
}
