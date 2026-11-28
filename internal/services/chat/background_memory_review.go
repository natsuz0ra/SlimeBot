package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/logging"
	llmsvc "slimebot/internal/services/llm"
)

func (s *ChatService) maybeTriggerMemoryReview(sessionID string, modelConfig llmsvc.ModelRuntimeConfig, userContent string, assistantContent string) {
	if s == nil || s.memory == nil || s.agent == nil || strings.TrimSpace(userContent) == "" || strings.TrimSpace(assistantContent) == "" {
		return
	}
	cfg, err := s.memory.Config(context.Background())
	if err != nil || !cfg.Enabled {
		if err != nil {
			logging.Warn("memory_review_config_failed", "session", sessionID, "err", err)
		}
		return
	}
	if !s.noteMemoryReviewTurn(sessionID, cfg.NudgeInterval) {
		return
	}
	go s.runMemoryReview(sessionID, modelConfig, userContent, assistantContent)
}

func (s *ChatService) noteMemoryReviewTurn(sessionID string, interval int) bool {
	if interval <= 0 {
		interval = 10
	}
	key := strings.TrimSpace(sessionID)
	if key == "" {
		return false
	}
	s.memoryReviewMu.Lock()
	defer s.memoryReviewMu.Unlock()
	s.memoryReviewTurns[key]++
	s.memoryReviewTouched[key] = time.Now()
	trigger := s.memoryReviewTurns[key] >= interval
	if trigger {
		s.memoryReviewTurns[key] = 0
	}
	if len(s.memoryReviewTurns) > 1024 {
		s.evictOldMemoryReviewSessionsLocked(256)
	}
	return trigger
}

func (s *ChatService) evictOldMemoryReviewSessionsLocked(maxEvict int) {
	for i := 0; i < maxEvict && len(s.memoryReviewTouched) > 0; i++ {
		var oldestSession string
		var oldestTime time.Time
		for sessionID, touchedAt := range s.memoryReviewTouched {
			if oldestSession == "" || touchedAt.Before(oldestTime) {
				oldestSession = sessionID
				oldestTime = touchedAt
			}
		}
		if oldestSession == "" {
			return
		}
		delete(s.memoryReviewTurns, oldestSession)
		delete(s.memoryReviewTouched, oldestSession)
	}
}

func (s *ChatService) runMemoryReview(sessionID string, modelConfig llmsvc.ModelRuntimeConfig, userContent string, assistantContent string) {
	ctx, cancel := context.WithTimeout(llmsvc.WithPrivacyScope(context.Background(), sessionID, "", "memory"), 2*time.Minute)
	defer cancel()
	if _, err := s.store.GetSessionByID(ctx, sessionID); err != nil {
		return
	}
	modelConfig.ThinkingLevel = "off"
	messages := []llmsvc.ChatMessage{
		{
			Role:    "system",
			Content: "You are a silent memory review worker. Decide whether the latest user/assistant turn contains durable facts worth saving. Use only the memory tool. Save concise factual statements, never commands. Do not save task progress, temporary todos, short-lived facts, IDs, or anything unsafe. If there is nothing durable to save, reply exactly NO_MEMORY_WRITES.",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Latest turn for review:\n\nUser:\n%s\n\nAssistant:\n%s", strings.TrimSpace(userContent), strings.TrimSpace(assistantContent)),
		},
	}
	_, err := s.agent.RunAgentLoop(ctx, modelConfig, sessionID, messages, nil, map[string]struct{}{}, AgentCallbacks{}, AgentLoopOptions{
		ApprovalMode: constants.ApprovalModeAuto,
		AllowedToolFunctions: map[string]struct{}{
			"memory__add":     {},
			"memory__replace": {},
			"memory__remove":  {},
			"memory__read":    {},
		},
	})
	if err != nil {
		logging.Warn("memory_review_failed", "session", sessionID, "err", err)
	}
}
