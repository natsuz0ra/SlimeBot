package chat

import (
	"strings"
	"sync"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/mcp"
	llmsvc "slimebot/internal/services/llm"
	memsvc "slimebot/internal/services/memory"
	plansvc "slimebot/internal/services/plan"
	skillsvc "slimebot/internal/services/skill"
)

// ChatService orchestrates the chat flow: context, agent, uploads, and per-session skills.
type ChatService struct {
	store          domain.ChatStore
	settingsStore  domain.SettingsStore
	agent          *AgentService
	skillRuntime   *skillsvc.SkillRuntimeService
	memory         *memsvc.MemoryService
	planService    *plansvc.PlanService
	uploads        *ChatUploadService
	titleGen       *titleGenerator
	skillsMu       sync.Mutex
	skillsBySess   map[string]map[string]struct{}
	skillTouchedAt map[string]time.Time
	promptMu       sync.RWMutex
	systemPrompt   string
	stablePrompt   string
	stableCatalog  string

	runContext RunContext

	platformModelMu sync.Mutex
	platformModelID string
	platformModelAt time.Time
}

// chatStreamAccumulator collects streamed text and the first push error, if any.
type chatStreamAccumulator struct {
	answerBuilder    strings.Builder
	narrationBuilder strings.Builder // plan mode: text before plan_start
	planBodyBuilder  strings.Builder // plan mode: text after plan_start
	planStarted      bool            // plan mode: set true when plan_start tool called
	pushErr          error
}

// ChatStreamResult is the outcome of one chat stream after persistence.
type ChatStreamResult struct {
	Answer            string
	IsInterrupted     bool
	IsStopPlaceholder bool
	SummaryUpdated    bool
	PushFailed        bool
	PushError         string
	PlanID            string
	Narration         string // text before the first heading in plan mode
	PlanBody          string // text from the first heading onwards in plan mode
}

// NewChatService constructs ChatService with per-session skill activation maps.
func NewChatService(store domain.ChatStore, settingsStore domain.SettingsStore, providerFactory *llmsvc.Factory, mcpManager *mcp.Manager, skillRuntime *skillsvc.SkillRuntimeService, memory *memsvc.MemoryService) *ChatService {
	s := &ChatService{
		store:          store,
		settingsStore:  settingsStore,
		skillRuntime:   skillRuntime,
		memory:         memory,
		titleGen:       newTitleGenerator(providerFactory, store),
		skillsBySess:   make(map[string]map[string]struct{}),
		skillTouchedAt: make(map[string]time.Time),
	}
	s.agent = NewAgentService(providerFactory, mcpManager, skillRuntime, memory)
	s.agent.SetSubagentHost(s)
	return s
}

// SetUploadService injects the upload staging service for one-turn consume/cleanup.
func (s *ChatService) SetUploadService(uploads *ChatUploadService) {
	s.uploads = uploads
}

// SetPlanService injects the plan service for plan mode file management.
func (s *ChatService) SetPlanService(ps *plansvc.PlanService) {
	s.planService = ps
}

// SetRunContext injects deployment/runtime info for the system prompt environment section.
func (s *ChatService) SetRunContext(ctx RunContext) {
	s.runContext = ctx
}

// getSessionActivatedSkills returns a copy of activated skills for the session.
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

// mergeSessionActivatedSkills merges newly activated skills and evicts LRU sessions if needed.
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

// evictOldSkillsSessionsLocked evicts least-recently-used skill sessions; caller holds skillsMu.
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

// getSystemPromptCached returns the in-memory system prompt cache.
func (s *ChatService) getSystemPromptCached() string {
	s.promptMu.RLock()
	defer s.promptMu.RUnlock()
	return s.systemPrompt
}

// setSystemPromptCached updates the in-memory system prompt cache.
func (s *ChatService) setSystemPromptCached(prompt string) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.systemPrompt = prompt
}

// getStableSystemPromptCached returns the stable system prompt and skill catalog snapshot.
func (s *ChatService) getStableSystemPromptCached() (prompt string, catalog string) {
	s.promptMu.RLock()
	defer s.promptMu.RUnlock()
	return s.stablePrompt, s.stableCatalog
}

// setStableSystemPromptCached updates stable system prompt and catalog snapshot.
func (s *ChatService) setStableSystemPromptCached(prompt string, catalog string) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.stablePrompt = prompt
	s.stableCatalog = catalog
}
