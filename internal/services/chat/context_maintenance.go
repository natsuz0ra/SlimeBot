package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"slimebot/internal/domain"
	contextsvc "slimebot/internal/services/context"
	llmsvc "slimebot/internal/services/llm"
)

type idleCompactionRequest struct {
	sessionID string
	modelID   string
}

type idleCompactionRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type ContextCompactionResult struct {
	ContextUsage
	Compacted bool `json:"compacted"`
}

// CompactContext synchronously compacts older history while holding the session turn lease.
func (s *ChatService) CompactContext(ctx context.Context, sessionID, modelID string) (ContextCompactionResult, error) {
	release, err := s.acquireTurnWithContext(ctx, sessionID)
	if err != nil {
		return ContextCompactionResult{}, err
	}
	defer release()
	usage, compacted, err := s.compactContext(ctx, sessionID, modelID, "manual")
	if err == nil {
		s.publishContextMaintenance(sessionID, usage, compacted)
	}
	return ContextCompactionResult{ContextUsage: usage, Compacted: compacted}, err
}

func (s *ChatService) compactContext(ctx context.Context, sessionID, modelID, reason string) (ContextUsage, bool, error) {
	if s.contexts == nil {
		return ContextUsage{}, false, fmt.Errorf("context maintenance is unavailable")
	}
	model, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return ContextUsage{}, false, err
	}
	config := llmsvc.ModelRuntimeConfig{
		ConfigID: model.ID, ProviderID: model.ProviderID, Provider: model.Provider,
		BaseURL: model.BaseURL, APIKey: model.APIKey, Model: model.Model, ContextSize: model.ContextSize,
	}
	history, err := s.store.ListAllSessionMessages(ctx, sessionID, -1)
	if err != nil {
		return ContextUsage{}, false, err
	}
	records, err := s.store.ListSessionToolCallRecordsByAssistantMessageIDs(ctx, sessionID, assistantMessageIDs(history))
	if err != nil {
		return ContextUsage{}, false, err
	}
	latestUser := -1
	for i := range history {
		if history[i].Role == "user" {
			latestUser = i
		}
	}
	if latestUser < 0 {
		return ContextUsage{}, false, fmt.Errorf("session has no user message to compact")
	}
	byAssistant := topLevelToolRecordsByAssistantID(records)
	seeds := make([]domain.ContextSeed, 0, len(history))
	for _, message := range history {
		seed := domain.ContextSeed{
			MessageID: message.ID, Seq: message.Seq, SourceHash: domain.ContextMessageHash(message),
			Fidelity: "legacy_replay", Messages: historyToChatMessages([]domain.Message{message}, byAssistant[message.ID]),
		}
		seeds = append(seeds, seed)
	}
	current := historyToChatMessages([]domain.Message{history[latestUser]}, byAssistant[history[latestUser].ID])
	if len(current) != 1 {
		return ContextUsage{}, false, fmt.Errorf("current user message cannot be prepared")
	}
	seeds[latestUser].Fidelity = "exact"
	seeds[latestUser].Messages = current

	prefix, err := s.buildStableContextPrefix(ctx)
	if err != nil {
		return ContextUsage{}, false, err
	}
	dynamic, err := s.buildDynamicContextTail(ctx, sessionID)
	if err != nil {
		return ContextUsage{}, false, err
	}
	prefix = append(prefix, dynamic...)
	requestID := uuid.NewString()
	run, err := s.contexts.Begin(ctx, sessionID, requestID, prefix, seeds)
	if err != nil {
		return ContextUsage{}, false, err
	}
	defer run.Close()
	runtimeState, err := s.contextRuntimeState(ctx, sessionID)
	if err != nil {
		return ContextUsage{}, false, err
	}
	run.SetRuntimeState(runtimeState)
	toolDefs, _, err := s.agent.buildRuntimeToolDefs(ctx, nil, 0)
	if err != nil {
		return ContextUsage{}, false, err
	}
	toolDefs = append(toolDefs, contextReadToolDef())
	prepared, err := run.PrepareWithReason(ctx, config, toolDefs, true, strings.TrimSpace(reason))
	if err != nil {
		return ContextUsage{}, false, err
	}
	result, err := s.buildContextMessagesDetailed(ctx, sessionID, config)
	if err != nil {
		return ContextUsage{}, false, err
	}
	result.usage.State = "ready"
	return result.usage, prepared.Compacted, nil
}

func (s *ChatService) publishContextMaintenance(sessionID string, usage ContextUsage, compacted bool) {
	background := s.backgrounds()
	background.mu.Lock()
	factory := background.factory
	background.mu.Unlock()
	if factory == nil {
		return
	}
	callbacks := factory(sessionID, func(event any) bool { return background.publish(sessionID, event) })
	if callbacks.OnContextUsage != nil {
		_ = callbacks.OnContextUsage(usage)
	}
	if compacted && callbacks.OnContextCompacted != nil {
		_ = callbacks.OnContextCompacted(usage)
	}
}

// maybeTriggerIdleCompaction queues near-pressure work; the turn release hook starts it.
func (s *ChatService) maybeTriggerIdleCompaction(sessionID string, config llmsvc.ModelRuntimeConfig, usage ContextUsage) {
	if s.contexts == nil || !s.contexts.Enabled() || config.ConfigID == "" || usage.UsedTokens <= 0 {
		return
	}
	budget, err := contextsvc.RequestBudget(config)
	if err != nil {
		return
	}
	nearPressure := max(1, budget.Pressure-max(256, budget.Window/10))
	if usage.UsedTokens < nearPressure {
		return
	}
	s.turnsMu.Lock()
	if s.idleCompactions == nil {
		s.idleCompactions = map[string]idleCompactionRequest{}
	}
	s.idleCompactions[sessionID] = idleCompactionRequest{sessionID: sessionID, modelID: config.ConfigID}
	busy := s.turns[sessionID]
	s.turnsMu.Unlock()
	if !busy {
		s.startIdleCompaction(sessionID)
	}
}

func (s *ChatService) acquireIdleTurn(parent context.Context, sessionID string) (context.Context, func(), error) {
	s.turnsMu.Lock()
	if s.turns[sessionID] {
		s.turnsMu.Unlock()
		return nil, nil, ErrTurnBusy
	}
	if s.cooperative != nil {
		if _, _, ok := s.cooperative.Runtime.CurrentRoot(sessionID); ok {
			s.turnsMu.Unlock()
			return nil, nil, ErrTurnBusy
		}
	}
	ctx, cancel := context.WithCancel(parent)
	run := &idleCompactionRun{cancel: cancel, done: make(chan struct{})}
	if s.idleCompactionRuns == nil {
		s.idleCompactionRuns = map[string]*idleCompactionRun{}
	}
	s.idleCompactionRuns[sessionID] = run
	s.turns[sessionID] = true
	s.turnsMu.Unlock()

	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			cancel()
			s.turnsMu.Lock()
			delete(s.turns, sessionID)
			if s.idleCompactionRuns[sessionID] == run {
				delete(s.idleCompactionRuns, sessionID)
			}
			close(run.done)
			s.turnsMu.Unlock()
			s.startIdleCompaction(sessionID)
		})
	}, nil
}

func (s *ChatService) startIdleCompaction(sessionID string) {
	s.turnsMu.Lock()
	request, ok := s.idleCompactions[sessionID]
	if ok {
		delete(s.idleCompactions, sessionID)
	}
	s.turnsMu.Unlock()
	if !ok {
		return
	}
	background := s.backgrounds()
	background.start(func() {
		ctx, release, err := s.acquireIdleTurn(background.ctx, sessionID)
		if err != nil {
			return
		}
		defer release()
		usage, compacted, err := s.compactContext(ctx, sessionID, request.modelID, "idle")
		if err != nil {
			return
		}
		s.publishContextMaintenance(sessionID, usage, compacted)
	})
}
