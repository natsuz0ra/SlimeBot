package services

import (
	"strings"

	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
)

// SessionService 负责会话领域编排，控制器仅处理协议转换。
type SessionService struct {
	store repositories.SessionStore
}

func NewSessionService(store repositories.SessionStore) *SessionService {
	return &SessionService{store: store}
}

func (s *SessionService) List() ([]models.Session, error) {
	return s.store.ListSessions()
}

func (s *SessionService) Create(name string) (*models.Session, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "新会话"
	}
	return s.store.CreateSession(trimmed)
}

func (s *SessionService) RenameByUser(id string, name string) error {
	return s.store.RenameSessionByUser(id, strings.TrimSpace(name))
}

func (s *SessionService) Delete(id string) error {
	return s.store.DeleteSession(id)
}

func (s *SessionService) ListMessages(sessionID string) ([]models.Message, error) {
	return s.store.ListSessionMessages(sessionID)
}

func (s *SessionService) ListToolCallRecords(sessionID string) ([]models.ToolCallRecord, error) {
	return s.store.ListSessionToolCallRecords(sessionID)
}

func (s *SessionService) SetModel(sessionID, modelConfigID string) error {
	return s.store.SetSessionModel(sessionID, strings.TrimSpace(modelConfigID))
}
