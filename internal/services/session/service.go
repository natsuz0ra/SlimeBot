package session

import (
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

func (s *SessionService) List() ([]domain.Session, error) {
	return s.store.ListSessions()
}

func (s *SessionService) Create(name string) (*domain.Session, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "New Chat"
	}
	return s.store.CreateSession(trimmed)
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

func (s *SessionService) ListMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]domain.Message, bool, error) {
	return s.store.ListSessionMessagesPage(sessionID, limit, before, after)
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
