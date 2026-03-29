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

// ChatService 负责聊天主流程编排，串联上下文构建、Agent 调用、附件和技能状态缓存。
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

// chatStreamAccumulator 聚合流式文本，并记住首次推送失败后的错误状态。
type chatStreamAccumulator struct {
	answerBuilder strings.Builder
	pushErr       error
}

// ChatStreamResult 描述一次聊天流结束后的最终结果与附加状态。
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

// NewChatService 创建聊天服务，并初始化会话级技能缓存。
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

// SetUploadService 注入附件暂存服务，供单轮消费与回收使用。
func (s *ChatService) SetUploadService(uploads *ChatUploadService) {
	s.uploads = uploads
}

// getSessionActivatedSkills 返回会话已激活技能的副本，避免调用方改写内部缓存。
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

// mergeSessionActivatedSkills 合并本轮新增技能，并在缓存过大时按最久未访问策略淘汰。
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

// evictOldSkillsSessionsLocked 淘汰最久未访问的技能会话缓存，调用方需已持有 skillsMu。
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

// getSystemPromptCached 读取已缓存的 system prompt。
func (s *ChatService) getSystemPromptCached() string {
	s.promptMu.RLock()
	defer s.promptMu.RUnlock()
	return s.systemPrompt
}

// setSystemPromptCached 更新内存中的 system prompt 缓存。
func (s *ChatService) setSystemPromptCached(prompt string) {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.systemPrompt = prompt
}
