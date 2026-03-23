package chat

import (
	"strings"
	"sync"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/mcp"
	memsvc "slimebot/internal/services/memory"
	oaisvc "slimebot/internal/services/openai"
	skillsvc "slimebot/internal/services/skill"
)

type ChatService struct {
	store            domain.ChatStore
	agent            *AgentService
	skillRuntime     *skillsvc.SkillRuntimeService
	memory           *memsvc.MemoryService
	uploads          *ChatUploadService
	skillsMu         sync.Mutex
	skillsBySess     map[string]map[string]struct{}
	skillTouchedAt   map[string]time.Time
	systemPromptPath string
	promptMu         sync.RWMutex
	systemPrompt     string

	platformModelMu sync.Mutex
	platformModelID string
	platformModelAt time.Time
}

type chatStreamAccumulator struct {
	answerBuilder strings.Builder
	pushErr       error
}

type ChatStreamResult struct {
	Answer            string
	IsInterrupted     bool
	IsStopPlaceholder bool
	TitleUpdated      bool
	Title             string
	SummaryUpdated    bool
	PushFailed        bool
	PushError         string
}

func NewChatService(store domain.ChatStore, openai *oaisvc.OpenAIClient, mcpManager *mcp.Manager, skillRuntime *skillsvc.SkillRuntimeService, memory *memsvc.MemoryService, systemPromptPath string) *ChatService {
	return &ChatService{
		store:            store,
		agent:            NewAgentService(openai, mcpManager, skillRuntime, memory),
		skillRuntime:     skillRuntime,
		memory:           memory,
		skillsBySess:     make(map[string]map[string]struct{}),
		skillTouchedAt:   make(map[string]time.Time),
		systemPromptPath: systemPromptPath,
	}
}

func (s *ChatService) SetUploadService(uploads *ChatUploadService) {
	s.uploads = uploads
}

func (s *ChatService) getSessionActivatedSkills(sessionID string) map[string]struct{} {
	if strings.TrimSpace(sessionID) == "" {
		return map[string]struct{}{}
	}
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if s.skillsBySess == nil {
		s.skillsBySess = make(map[string]map[string]struct{})
	}
	if s.skillTouchedAt == nil {
		s.skillTouchedAt = make(map[string]time.Time)
	}
	current := s.skillsBySess[sessionID]
	s.skillTouchedAt[sessionID] = time.Now()
	copyMap := make(map[string]struct{}, len(current))
	for name := range current {
		copyMap[name] = struct{}{}
	}
	return copyMap
}

func (s *ChatService) mergeSessionActivatedSkills(sessionID string, activated map[string]struct{}) {
	if strings.TrimSpace(sessionID) == "" || len(activated) == 0 {
		return
	}
	s.skillsMu.Lock()
	defer s.skillsMu.Unlock()
	if s.skillsBySess == nil {
		s.skillsBySess = make(map[string]map[string]struct{})
	}
	if s.skillTouchedAt == nil {
		s.skillTouchedAt = make(map[string]time.Time)
	}
	existing := s.skillsBySess[sessionID]
	if existing == nil {
		existing = make(map[string]struct{}, len(activated))
		s.skillsBySess[sessionID] = existing
	}
	for name := range activated {
		existing[name] = struct{}{}
	}
	s.skillTouchedAt[sessionID] = time.Now()
	if len(s.skillsBySess) > 1024 {
		s.evictOldSkillsSessionsLocked(256)
	}
}

func (s *ChatService) evictOldSkillsSessionsLocked(maxEvict int) {
	for i := 0; i < maxEvict && len(s.skillTouchedAt) > 0; i++ {
		var oldestSession string
		var oldestTime time.Time
		for sessionID, touchedAt := range s.skillTouchedAt {
			if oldestSession == "" || touchedAt.Before(oldestTime) {
				oldestSession = sessionID
				oldestTime = touchedAt
			}
		}
		if oldestSession == "" {
			return
		}
		delete(s.skillsBySess, oldestSession)
		delete(s.skillTouchedAt, oldestSession)
	}
}

func (s *ChatService) getSystemPromptCached() string {
	s.promptMu.RLock()
	defer s.promptMu.RUnlock()
	return s.systemPrompt
}

func (s *ChatService) setSystemPromptCached(prompt string) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.systemPrompt = prompt
}
