package session

import (
	"context"
	"strings"
	"time"

	"slimebot/internal/domain"
)

// SessionService 负责会话领域编排，控制器仅处理协议转换。
type SessionService struct {
	store domain.SessionStore
}

func NewSessionService(store domain.SessionStore) *SessionService {
	return &SessionService{store: store}
}

func (s *SessionService) List(limit, offset int, query string) ([]domain.Session, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.ListSessions(limit, offset, strings.TrimSpace(query))
}

func (s *SessionService) Create(name string) (*domain.Session, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "New Chat"
	}
	return s.store.CreateSession(context.Background(), trimmed)
}

func (s *SessionService) RenameByUser(id string, name string) error {
	return s.store.RenameSessionByUser(id, strings.TrimSpace(name))
}

func (s *SessionService) Delete(id string) error {
	return s.store.DeleteSession(id)
}

func (s *SessionService) ListMessages(sessionID string) ([]domain.Message, error) {
	return s.store.ListSessionMessages(sessionID)
}

func (s *SessionService) ListMessagesPage(sessionID string, limit int, before *time.Time, beforeSeq *int64, after *time.Time, afterSeq *int64) ([]domain.Message, bool, error) {
	return s.store.ListSessionMessagesPage(sessionID, limit, before, beforeSeq, after, afterSeq)
}

func (s *SessionService) ListToolCallRecords(sessionID string) ([]domain.ToolCallRecord, error) {
	return s.store.ListSessionToolCallRecords(sessionID)
}

func (s *SessionService) ListToolCallRecordsByAssistantMessageIDs(sessionID string, messageIDs []string) ([]domain.ToolCallRecord, error) {
	return s.store.ListSessionToolCallRecordsByAssistantMessageIDs(sessionID, messageIDs)
}

func (s *SessionService) SetModel(sessionID, modelConfigID string) error {
	return s.store.SetSessionModel(sessionID, strings.TrimSpace(modelConfigID))
}
