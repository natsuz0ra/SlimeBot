package chat

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	sandboxpolicy "slimebot/internal/sandbox"
	contextsvc "slimebot/internal/services/context"
	llmsvc "slimebot/internal/services/llm"
	usagesvc "slimebot/internal/services/usage"
	"slimebot/internal/tools"
)

type turnEngineCapabilities struct {
	localContext   bool
	skillLearning  bool
	memoryReview   bool
	idleCompaction bool
}

type turnEngine interface {
	capabilities() turnEngineCapabilities
	prepare(ctx context.Context, s *ChatService, state *chatTurnState, req ChatTurnRequest) error
	run(ctx context.Context, s *ChatService, turn *chatTurn, approvalMode string, callbacks AgentCallbacks) (*chatTurnResult, error)
	emptyAnswer() string
}

func (s *ChatService) resolveTurnEngine(session *domain.Session, modelID string) (turnEngine, error) {
	agentID, err := s.acpRoute(session, modelID)
	if err != nil {
		return nil, err
	}
	if agentID != "" {
		return externalACPTurnEngine{agentID: agentID}, nil
	}
	return localTurnEngine{}, nil
}

type localTurnEngine struct{}

func (localTurnEngine) capabilities() turnEngineCapabilities {
	return turnEngineCapabilities{localContext: true, skillLearning: true, memoryReview: true, idleCompaction: true}
}

func (localTurnEngine) emptyAnswer() string { return "The model returned no content." }

func (localTurnEngine) prepare(ctx context.Context, s *ChatService, state *chatTurnState, req ChatTurnRequest) error {
	model, err := s.ResolveModelRuntimeConfig(ctx, req.ModelID)
	if err != nil {
		return err
	}
	model.ThinkingLevel = req.ThinkingLevel
	if err := s.unbindACPForLocalTurn(ctx, state.session); err != nil {
		return err
	}

	var user *domain.Message
	if req.EditMessageID != "" {
		user, err = s.store.UpdateUserMessageAndPruneAfter(ctx, req.SessionID, req.EditMessageID, req.Content)
		if err != nil {
			return err
		}
		state.modelUser = historyToChatMessages([]domain.Message{*user}, nil)[0]
		state.userContent = strings.TrimSpace(user.Content)
		state.userCreatedAt = user.CreatedAt
	} else {
		state.attachments, err = s.resolveTurnAttachments(req.SessionID, req.AttachmentIDs)
		if err != nil {
			return err
		}
		state.userContent = req.Content
		state.modelUser = llmsvc.ChatMessage{Role: "user"}
		if len(state.attachments) > 0 {
			parts, fallback := buildUserMessageContentParts(state.userContent, state.attachments)
			state.modelUser.ContentParts = parts
			if len(parts) == 0 || len(fallback) > 0 {
				state.userContent = buildUserPromptWithAttachments(state.userContent, state.attachments)
			}
		}
		state.modelUser.Content = state.userContent
		user, err = s.persistUserMessage(ctx, req.SessionID, req.Content, req.DisplayContent, req.ReceivedAt, state.attachments)
		if err != nil {
			return err
		}
		state.userCreatedAt = req.ReceivedAt
	}

	// Context and enabled MCP configuration can be loaded independently.
	var history contextBuildResult
	var mcps []domain.MCPConfig
	var contextErr, mcpErr error
	var prepareWG sync.WaitGroup
	prepareWG.Add(2)
	go func() {
		defer prepareWG.Done()
		history, contextErr = s.buildContextMessagesDetailed(ctx, req.SessionID, model)
	}()
	go func() {
		defer prepareWG.Done()
		mcps, mcpErr = s.store.ListEnabledMCPConfigs(ctx)
	}()
	prepareWG.Wait()
	if contextErr != nil {
		return contextErr
	}
	if mcpErr != nil {
		return mcpErr
	}

	if req.EditMessageID == "" {
		if len(state.modelUser.ContentParts) > 0 {
			overrideLatestUserTurnWithParts(history.messages, state.userContent, state.modelUser.ContentParts)
		} else if len(state.attachments) > 0 || strings.TrimSpace(req.DisplayContent) != "" {
			overrideLatestUserTurn(history.messages, state.userContent)
		}
	}
	state.userMessageID = user.ID
	state.modelConfig = model
	state.contextMessages = history.messages
	state.contextUsage = history.usage
	state.contextCompacted = history.compactedNow
	state.enabledMCPConfigs = mcps
	return nil
}

func (localTurnEngine) run(ctx context.Context, s *ChatService, turn *chatTurn, approvalMode string, agentCallbacks AgentCallbacks) (*chatTurnResult, error) {
	req, state := turn.request, turn.state
	sessionID, requestID := req.SessionID, req.RequestID
	planMode, subagentModelID := req.PlanMode, req.SubagentModelID
	callbacks := turn.events
	usageTracker := newContextUsageTracker(state.contextUsage, callbacks.OnContextUsage)
	requestTeamRuntime := newTeamRuntime(s.teamService, sessionID, requestID)
	activatedSkills := s.getSessionActivatedSkills(sessionID)
	previouslyActivated := make(map[string]struct{}, len(activatedSkills))
	for id := range activatedSkills {
		previouslyActivated[id] = struct{}{}
	}
	sandboxPolicy, err := s.resolveSandboxPolicy(ctx, state.session.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	if constants.ClientSurfaceFromContext(ctx) != constants.ClientSurfaceCLI {
		ctx = sandboxpolicy.WithWorkingDirectory(ctx, state.session.WorkingDirectory)
	}
	ctx = usagesvc.WithScope(ctx, usagesvc.Scope{RootSessionID: sessionID, SessionID: sessionID, RequestID: requestID, WorkingDirectory: state.session.WorkingDirectory})

	finishCheckpoint, err := s.beginFileCheckpoint(ctx, state, requestID, sandboxPolicy)
	if err != nil {
		return nil, err
	}
	defer finishCheckpoint()

	ctx = tools.WithBackgroundObserver(ctx, s.backgroundObserver(ctx, state, subagentModelID, approvalMode, planMode))
	var cooperative *cooperativeLoop
	if s.cooperative != nil {
		var rootErr error
		ctx, rootErr = s.cooperative.Runtime.BeginRoot(ctx, sessionID, requestID)
		if rootErr != nil {
			return nil, rootErr
		}
		childModel := state.modelConfig
		if subagentModelID != "" {
			childModel, rootErr = s.ResolveModelRuntimeConfig(ctx, subagentModelID)
			if rootErr != nil {
				return nil, rootErr
			}
		}
		caller := subagentCaller(sessionID, requestID, approvalMode, planMode)
		caller.ThinkingLevel = state.modelConfig.ThinkingLevel
		if s.settingsStore != nil {
			if value, e := s.settingsStore.GetSetting(ctx, "SUBAGENT_MAX_DEPTH"); e == nil && value != "" {
				if n, e := strconv.Atoi(value); e == nil && n >= 0 && n <= 4 {
					caller.MaxDepth = n
				}
			}
		}
		cooperative = &cooperativeLoop{service: s.cooperative, caller: caller, model: childModel, workspace: state.session.WorkingDirectory, plan: planMode}
	}
	contextRun, err := s.beginContextRun(ctx, sessionID, requestID, state)
	if err != nil {
		return nil, err
	}
	defer contextRun.Close()
	contextRun.OnStatus = func(phase string) error {
		usage := state.contextUsage
		usage.State = phase
		if callbacks.OnContextUsage != nil {
			return callbacks.OnContextUsage(usage)
		}
		return nil
	}
	if cooperative != nil {
		agentCallbacks = s.cooperativeApprovals(ctx, cooperative.caller, requestID, agentCallbacks)
	}
	agentStart := time.Now()
	var planCompleted bool
	var latestUsage llmsvc.TokenUsage
	answer, err := s.agent.RunAgentLoop(ctx, state.modelConfig, sessionID, state.contextMessages, state.enabledMCPConfigs, activatedSkills, agentCallbacks, AgentLoopOptions{
		Cooperative:     cooperative,
		steering:        func() *turnInbox { v, _ := ctx.Value(turnInboxKey{}).(*turnInbox); return v }(),
		ApprovalMode:    approvalMode,
		PlanMode:        planMode,
		PlanComplete:    &planCompleted,
		SubagentModelID: subagentModelID,
		LatestUsage:     &latestUsage,
		OnProviderUsage: func(usage llmsvc.TokenUsage) error {
			return usageTracker.setUsedTokens(usage.InputContextTokens(state.modelConfig.Provider))
		},
		ContextRun: contextRun,
		OnPrepared: func(p contextsvc.Prepared) error {
			if p.CompactedAt == "" {
				p.CompactedAt = state.contextUsage.CompactedAt
			}
			usage := buildContextUsage(sessionID, state.modelConfig, p.Messages, nil, nil, state.contextUsage.IsCompacted || p.Compacted, p.CompactedAt)
			usage.InputBudget = p.Budget.HardInput
			usage.OutputReserve = p.Budget.Output
			usage.CompactionBeforeTokens = state.contextUsage.CompactionBeforeTokens
			usage.CompactionAfterTokens = state.contextUsage.CompactionAfterTokens
			usage.CompactionReason = state.contextUsage.CompactionReason
			if p.Compacted {
				usage.CompactionBeforeTokens = p.CompactionBeforeTokens
				usage.CompactionAfterTokens = p.CompactionAfterTokens
				usage.CompactionReason = p.Reason
			}
			usage.UsedTokens = p.InputTokens
			toolTokens := p.InputTokens - contextsvc.Estimate(p.Messages, nil)
			if toolTokens > 0 {
				usage.Layers = upsertContextLayerUsage(usage.Layers, "tools", toolTokens)
			}
			usage = normalizeContextUsagePercentages(usage)
			s.attachOutputBudget(ctx, sessionID, &usage)
			state.contextUsage = usage
			usageTracker.mu.Lock()
			usageTracker.usage = usage
			usageTracker.mu.Unlock()
			if callbacks.OnContextUsage != nil {
				if err := callbacks.OnContextUsage(usage); err != nil {
					return err
				}
			}
			if p.Compacted && callbacks.OnContextCompacted != nil {
				return callbacks.OnContextCompacted(usage)
			}
			return nil
		},
		SandboxPolicy: sandboxPolicy,
		teamRuntime:   requestTeamRuntime,
	})
	logging.Span("agent_loop", agentStart)
	state.activatedSkillIDs = state.activatedSkillIDs[:0]
	for id := range activatedSkills {
		if _, existed := previouslyActivated[id]; !existed {
			state.activatedSkillIDs = append(state.activatedSkillIDs, id)
		}
	}
	sort.Strings(state.activatedSkillIDs)
	s.mergeSessionActivatedSkills(sessionID, activatedSkills)
	return &chatTurnResult{
		answer: answer, planCompleted: planCompleted,
		tokenUsage: nonZeroTokenUsage(latestUsage), teamRuntime: requestTeamRuntime,
	}, err
}
