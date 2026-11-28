package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	sandboxpolicy "slimebot/internal/sandbox"
	llmsvc "slimebot/internal/services/llm"
	usagesvc "slimebot/internal/services/usage"
)

const (
	toolCallMarkerFmt   = "\n<!-- TOOL_CALL:%s -->\n"
	thinkingMarkerFmt   = "\n<!-- THINKING:%s -->\n"
	planStartMarker     = "\n<!-- PLAN_START -->\n"
	planEndMarker       = "\n<!-- PLAN_END -->\n"
	turnFinalizeTimeout = 10 * time.Second
)

// StripContentMarkers removes TOOL_CALL/PLAN markers from text for real-time display.
func StripContentMarkers(input string) string {
	return domain.StripContentMarkers(input)
}

// ChatTurnRequest is the input shared by all chat entry points.
type ChatTurnRequest struct {
	SessionID            string
	RequestID            string
	ReceivedAt           time.Time
	Content              string
	DisplayContent       string
	ModelID              string
	AttachmentIDs        []string
	ThinkingLevel        string
	PlanMode             bool
	SubagentModelID      string
	ApprovalModeOverride string
	EditMessageID        string
}

type chatTurnState struct {
	engine            turnEngine
	userMessageID     string
	modelUser         llmsvc.ChatMessage
	contextRequestID  string
	session           *domain.Session
	modelConfig       llmsvc.ModelRuntimeConfig
	contextMessages   []llmsvc.ChatMessage
	contextUsage      ContextUsage
	contextCompacted  bool
	enabledMCPConfigs []domain.MCPConfig
	attachments       []UploadedAttachment
	userContent       string
	userCreatedAt     time.Time
	activatedSkillIDs []string
}

type chatTurnResult struct {
	answer        string
	interrupted   bool
	executionErr  error
	planCompleted bool
	narration     string
	planBody      string
	tokenUsage    *llmsvc.TokenUsage
	teamRuntime   *teamRuntime
}

type chatTurn struct {
	request  ChatTurnRequest
	state    *chatTurnState
	recorder *TurnRecorder
	sink     *EventSink
	events   AgentCallbacks
}

func (s *ChatService) newChatTurn(req ChatTurnRequest, state *chatTurnState, callbacks AgentCallbacks) *chatTurn {
	sink := &EventSink{}
	return &chatTurn{
		request:  req,
		state:    state,
		recorder: &TurnRecorder{store: s.store, sessionID: req.SessionID, requestID: req.RequestID, thinkings: make(map[thinkingScope]string)},
		sink:     sink,
		events:   sink.adapt(callbacks),
	}
}

func (s *ChatService) HandleChatStream(
	ctx context.Context,
	sessionID, requestID, content, displayContent, modelID string,
	attachmentIDs []string,
	thinkingLevel string,
	planMode bool,
	subagentModelID, approvalModeOverride string,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	return s.HandleChatStreamWithReceivedAt(ctx, sessionID, requestID, time.Time{}, content, displayContent, modelID, attachmentIDs, thinkingLevel, planMode, subagentModelID, approvalModeOverride, callbacks)
}

func (s *ChatService) HandleChatStreamWithReceivedAt(
	ctx context.Context,
	sessionID, requestID string,
	receivedAt time.Time,
	content, displayContent, modelID string,
	attachmentIDs []string,
	thinkingLevel string,
	planMode bool,
	subagentModelID, approvalModeOverride string,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	return s.RunTurn(ctx, ChatTurnRequest{
		SessionID: sessionID, RequestID: requestID, ReceivedAt: receivedAt,
		Content: content, DisplayContent: displayContent, ModelID: modelID, AttachmentIDs: attachmentIDs,
		ThinkingLevel: thinkingLevel, PlanMode: planMode, SubagentModelID: subagentModelID, ApprovalModeOverride: approvalModeOverride,
	}, callbacks)
}

func (s *ChatService) HandleEditedChatStream(
	ctx context.Context,
	sessionID, requestID, messageID, content, modelID, thinkingLevel string,
	planMode bool,
	subagentModelID, approvalModeOverride string,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message %s: %w", messageID, apperrors.ErrNotFound)
	}
	return s.RunTurn(ctx, ChatTurnRequest{
		SessionID: sessionID, RequestID: requestID, EditMessageID: messageID, Content: content, ModelID: modelID,
		ThinkingLevel: thinkingLevel, PlanMode: planMode, SubagentModelID: subagentModelID, ApprovalModeOverride: approvalModeOverride,
	}, callbacks)
}

// RunTurn prepares, executes and records a new or edited chat turn.
func (s *ChatService) RunTurn(ctx context.Context, req ChatTurnRequest, callbacks AgentCallbacks) (*ChatStreamResult, error) {
	if req.EditMessageID == "" && strings.TrimSpace(req.Content) == "" && len(req.AttachmentIDs) == 0 {
		return nil, fmt.Errorf("Message cannot be empty.")
	}
	if req.ReceivedAt.IsZero() {
		req.ReceivedAt = time.Now()
	}
	sessionID, requestID := req.SessionID, req.RequestID
	ctx = llmsvc.WithPrivacyScope(ctx, sessionID, requestID, "chat")
	ctx = usagesvc.WithScope(ctx, usagesvc.Scope{RootSessionID: sessionID, SessionID: sessionID, RequestID: requestID, WorkingDirectory: sandboxpolicy.WorkingDirectoryFromContext(ctx)})

	ctx, endTurn, err := s.beginTurn(ctx, sessionID, requestID)
	if err != nil {
		return nil, err
	}
	defer endTurn()
	session, err := s.store.GetSessionByID(ctx, sessionID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, fmt.Errorf("Session not found: %s.", sessionID)
	}
	if err != nil {
		return nil, err
	}
	engine, err := s.resolveTurnEngine(session, req.ModelID)
	if err != nil {
		return nil, err
	}
	state := &chatTurnState{engine: engine, session: session}
	defer func() { s.cleanupTurnAttachments(state.attachments) }()
	if err := engine.prepare(ctx, s, state, req); err != nil {
		return nil, err
	}

	turn := s.newChatTurn(req, state, callbacks)
	events := turn.events
	localContext := engine.capabilities().localContext
	if req.EditMessageID != "" && events.OnMessageEdited != nil {
		_ = events.OnMessageEdited(req.EditMessageID, state.userContent, state.userCreatedAt)
	}
	if localContext {
		ctx.Value(turnInboxKey{}).(*turnInbox).open()
	}
	if events.OnTurnStarted != nil {
		_ = events.OnTurnStarted(requestID)
	}
	if localContext && events.OnContextUsage != nil {
		_ = events.OnContextUsage(state.contextUsage)
	}
	if state.contextCompacted && events.OnContextCompacted != nil {
		_ = events.OnContextCompacted(state.contextUsage)
	}
	if localContext && req.EditMessageID == "" {
		s.maybeGenerateTitleAsync(state.session, state.modelConfig, state.userContent, events.TitlePrefix, events.OnTitleGenerated)
	}
	if localContext && req.PlanMode {
		state.contextMessages = append(state.contextMessages, llmsvc.ChatMessage{Role: "system", Content: planModeSystemMessage})
	}

	// Keep the root live until its assistant message is persisted.
	var interrupted bool
	failed := true
	if localContext {
		defer func() { s.finishChatRoot(ctx, sessionID, requestID, interrupted, failed) }()
	}
	result, err := s.executeChatTurn(ctx, turn)
	if err != nil {
		if localContext && errors.Is(err, usagesvc.ErrSessionOutputTokenBudgetExceeded) && events.OnContextUsage != nil {
			s.attachOutputBudget(ctx, sessionID, &state.contextUsage)
			_ = events.OnContextUsage(state.contextUsage)
		}
		return nil, err
	}
	stream, err := s.finalizeChatTurn(ctx, turn, result)
	interrupted = result.interrupted
	failed = err != nil || result.executionErr != nil
	return stream, err
}

func (s *ChatService) finishChatRoot(ctx context.Context, sessionID, requestID string, interrupted, failed bool) {
	if s.cooperative == nil {
		return
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	status := "completed"
	if failed {
		status = "failed"
	}
	if interrupted || ctx.Err() != nil {
		status = "canceled"
		_ = s.agent.getSessionProcessManager(sessionID).StopAndWait(finishCtx)
	}
	_ = s.cooperative.Runtime.FinishRootStatus(finishCtx, requestID, status)
}

// executeChatTurn wires reliable recording to the selected engine and collects its output.
func (s *ChatService) executeChatTurn(ctx context.Context, turn *chatTurn) (*chatTurnResult, error) {
	req, state := turn.request, turn.state
	localContext := state.engine.capabilities().localContext
	localPlan := req.PlanMode && localContext
	accumulator := &chatStreamAccumulator{}
	events := turn.events
	recorder := turn.recorder
	var streamMu sync.Mutex
	streamStart := time.Now()
	var firstTokenAt time.Time

	callbacks := events
	callbacks.turnRecordingError = recorder.failure
	callbacks.OnChunk = func(chunk string) error {
		if chunk == "" {
			return nil
		}
		streamMu.Lock()
		if firstTokenAt.IsZero() {
			firstTokenAt = time.Now()
		}
		accumulator.answerBuilder.WriteString(chunk)
		planStarted := accumulator.planStarted
		if localPlan {
			if planStarted {
				accumulator.planBodyBuilder.WriteString(chunk)
			} else {
				accumulator.narrationBuilder.WriteString(chunk)
			}
		}
		streamMu.Unlock()
		if planStarted {
			if events.OnPlanChunk != nil {
				return events.OnPlanChunk(chunk)
			}
		} else if events.OnChunk != nil {
			return events.OnChunk(chunk)
		}
		return nil
	}
	callbacks.OnToolCallStart = func(req ApprovalRequest) error {
		if err := recorder.startTool(ctx, req, toolStartStatus(req)); err != nil {
			return err
		}
		streamMu.Lock()
		accumulator.answerBuilder.WriteString(fmt.Sprintf(toolCallMarkerFmt, req.ToolCallID))
		streamMu.Unlock()
		if events.OnToolCallStart != nil {
			if localPlan {
				req.Preamble = ""
			}
			return events.OnToolCallStart(req)
		}
		return nil
	}
	callbacks.OnToolApprovalRequired = func(req ApprovalRequest) error {
		if err := recorder.startTool(ctx, req, constants.ToolCallStatusPending); err != nil {
			return err
		}
		if events.OnToolApprovalRequired != nil {
			return events.OnToolApprovalRequired(req)
		}
		return nil
	}
	callbacks.OnToolCallResult = func(result ToolCallResult) error {
		if err := recorder.finishTool(ctx, result); err != nil {
			return err
		}
		if events.OnToolCallResult != nil {
			return events.OnToolCallResult(result)
		}
		return nil
	}
	callbacks.OnThinkingStart = func(meta ThinkingEventMeta) error {
		id, err := recorder.startThinking(ctx, meta)
		if err != nil {
			return err
		}
		if meta.ParentToolCallID == "" && meta.SubagentRunID == "" {
			streamMu.Lock()
			accumulator.answerBuilder.WriteString(fmt.Sprintf(thinkingMarkerFmt, id))
			streamMu.Unlock()
		}
		if events.OnThinkingStart != nil {
			return events.OnThinkingStart(meta)
		}
		return nil
	}
	callbacks.OnThinkingChunk = func(chunk string, meta ThinkingEventMeta) error {
		if err := recorder.appendThinking(ctx, meta, chunk); err != nil {
			return err
		}
		if events.OnThinkingChunk != nil {
			return events.OnThinkingChunk(chunk, meta)
		}
		return nil
	}
	callbacks.OnThinkingDone = func(meta ThinkingEventMeta) error {
		if err := recorder.finishThinking(ctx, meta); err != nil {
			return err
		}
		if events.OnThinkingDone != nil {
			return events.OnThinkingDone(meta)
		}
		return nil
	}
	callbacks.OnPlanStart = func() error {
		streamMu.Lock()
		accumulator.planStarted = true
		accumulator.answerBuilder.WriteString(planStartMarker)
		streamMu.Unlock()
		return nil
	}

	approvalMode := strings.TrimSpace(req.ApprovalModeOverride)
	if approvalMode == "" {
		approvalMode = constants.ApprovalModeStandard
		if s.settingsStore != nil {
			if mode, err := s.settingsStore.GetSetting(ctx, constants.SettingApprovalMode); err == nil && mode != "" {
				approvalMode = mode
			}
		}
	}

	result, runErr := state.engine.run(ctx, s, turn, approvalMode, callbacks)
	if err := recorder.failure(); err != nil {
		return nil, err
	}
	canceled := ctx.Err() != nil || errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)
	if result == nil {
		if !canceled {
			return nil, runErr
		}
		result = &chatTurnResult{}
	}
	result.interrupted = result.interrupted || canceled
	if !result.interrupted {
		if err := recorder.finishThinkings(ctx); err != nil {
			if ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
				return nil, err
			}
			result.interrupted = true
		}
	}
	if runErr != nil && !result.interrupted {
		if result.answer == "" && (localContext || strings.TrimSpace(accumulator.answerBuilder.String()) == "") {
			return nil, runErr
		}
		result.executionErr = runErr
	}

	firstTokenMs := int64(-1)
	if !firstTokenAt.IsZero() {
		firstTokenMs = firstTokenAt.Sub(streamStart).Milliseconds()
	}
	logging.Info("chat_stream_done", "session", req.SessionID, "first_token_ms", firstTokenMs, "total_stream_ms", time.Since(streamStart).Milliseconds())

	if localPlan {
		if accumulator.planStarted {
			accumulator.answerBuilder.WriteString(planEndMarker)
			result.answer = strings.TrimSpace(accumulator.answerBuilder.String())
			result.narration = strings.TrimSpace(accumulator.narrationBuilder.String())
			result.planBody = accumulator.planBodyBuilder.String()
		} else {
			result.answer = strings.TrimSpace(accumulator.answerBuilder.String())
			result.narration, result.planBody = splitNarrationAndPlan(result.answer)
		}
		if result.planBody != "" && events.OnPlanBody != nil {
			_ = events.OnPlanBody(result.planBody)
		}
	} else if accumulated := strings.TrimSpace(accumulator.answerBuilder.String()); accumulated != "" {
		result.answer = accumulated
	}
	if strings.TrimSpace(result.answer) == "" && !result.interrupted {
		result.answer = state.engine.emptyAnswer()
	}
	return result, nil
}

// finalizeChatTurn commits the assistant and its records before running follow-up work.
func (s *ChatService) finalizeChatTurn(ctx context.Context, turn *chatTurn, result *chatTurnResult) (*ChatStreamResult, error) {
	req, state := turn.request, turn.state
	sessionID, requestID := req.SessionID, req.RequestID
	if result.interrupted || ctx.Err() != nil {
		result.interrupted = true
		finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), turnFinalizeTimeout)
		defer cancel()
		ctx = finalCtx
	}
	ctx = llmsvc.WithPrivacyScope(ctx, sessionID, requestID, "chat")
	completed, err := turn.recorder.complete(ctx, domain.CompleteChatTurnInput{
		SessionID: sessionID, RequestID: requestID, ContextRequestID: state.contextRequestID,
		Content: result.answer, Interrupted: result.interrupted, TokenUsage: result.tokenUsage,
	})
	if err != nil {
		return nil, err
	}

	assistant := completed.AssistantMessage
	streamResult := &ChatStreamResult{
		Answer:            assistant.Content,
		IsInterrupted:     assistant.IsInterrupted,
		IsStopPlaceholder: assistant.IsStopPlaceholder,
	}
	defer func() {
		if pushErr := turn.sink.failure(); pushErr != nil {
			streamResult.PushFailed = true
			streamResult.PushError = pushErr.Error()
		}
	}()
	if completed.Deduplicated {
		return streamResult, nil
	}
	streamResult.Narration = result.narration
	streamResult.PlanBody = result.planBody
	capabilities := state.engine.capabilities()
	if result.executionErr != nil && !capabilities.localContext {
		streamResult.ExecutionError = result.executionErr.Error()
	}
	planMode := req.PlanMode && capabilities.localContext
	if result.teamRuntime != nil {
		if err := result.teamRuntime.finalize(ctx, assistant.ID, result.interrupted, turn.events); err != nil {
			return nil, err
		}
	}
	if planMode && !result.interrupted && s.planService != nil && strings.TrimSpace(result.planBody) != "" {
		if !result.planCompleted {
			logging.Info("plan_not_submitted", "session", sessionID)
		} else {
			planContent := strings.TrimSpace(result.planBody)
			title := "Plan"
			for _, line := range strings.Split(planContent, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "# ") {
					title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
					break
				}
			}
			if plan, saveErr := s.planService.SavePlan(sessionID, title, planContent); saveErr != nil {
				logging.Info("plan_save_error", "session", sessionID, "error", saveErr.Error())
			} else {
				logging.Info("plan_saved", "session", sessionID)
				streamResult.PlanID = plan.ID
			}
		}
	}

	if capabilities.skillLearning {
		streamResult.SkillLearning = s.maybeLearnSkill(ctx, sessionID, requestID, assistant.ID, state, result, planMode)
	}
	if !planMode && !result.interrupted && strings.TrimSpace(result.answer) != "" {
		if capabilities.memoryReview {
			s.maybeTriggerMemoryReview(sessionID, state.modelConfig, state.userContent, result.answer)
		}
		if capabilities.idleCompaction {
			s.maybeTriggerIdleCompaction(sessionID, state.modelConfig, state.contextUsage)
		}
	}
	if capabilities.localContext && turn.events.OnContextUsage != nil {
		s.attachOutputBudget(ctx, sessionID, &state.contextUsage)
		_ = turn.events.OnContextUsage(state.contextUsage)
	}
	return streamResult, nil
}

// splitNarrationAndPlan splits accumulated plan text into narration (before first heading)
// and plan body (from first heading onwards). Returns (narration, planBody).
// If no heading is found, returns ("", fullText).
func splitNarrationAndPlan(fullText string) (string, string) {
	lines := strings.Split(fullText, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			narration := strings.TrimSpace(strings.Join(lines[:i], "\n"))
			planBody := strings.Join(lines[i:], "\n")
			return narration, planBody
		}
	}
	return "", fullText
}

const planModeSystemMessage = `Plan mode is active. The user indicated that they do not want you to execute yet — you MUST NOT make any edits, run any commands, or otherwise make any changes to the system. This supersedes any other instructions you have received.

## Your Task

Analyze the user's request and create a detailed implementation plan.

## Workflow

1. **Research** — Use read-only tools (file_read, grep, glob, web_search, web_extract, skills) ONLY to gather information.
2. **Analyze** — Assess the current state and identify what needs to change.
3. **Begin Plan** — Call the plan_start tool when you are ready to begin writing your plan. All text output BEFORE this call will appear as narration; all text AFTER will be the plan body. You MUST call this before writing your plan.
4. **Plan** — Create a structured markdown plan with:
   - **Background**: Context and motivation
   - **Analysis**: Current state assessment
   - **Steps**: Numbered implementation steps with file paths and code references
   - **Risks**: Potential issues and mitigations
   - **Expected Outcome**: What success looks like
5. **Submit** — When your complete plan has been written, call the plan_complete__submit tool. You MUST call this tool when finished — without it the user will not see the review menu.

## Rules

- You MUST NOT write implementation code — only describe what needs to be done.
- You MUST NOT execute any commands or modify any files.
- Be specific: include file paths, function names, and concrete actions in each step.
- You MUST call plan_start before writing your plan.
- You MUST call plan_complete__submit when your plan is complete.
- Only file_read, grep, glob, web_search, web_extract, skills, plan_start, and plan_complete__submit tools are available.`
