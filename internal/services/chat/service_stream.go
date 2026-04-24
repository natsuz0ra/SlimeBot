package chat

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slimebot/internal/apperrors"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"

	"github.com/google/uuid"
)

const (
	toolCallMarkerFmt = "\n<!-- TOOL_CALL:%s -->\n"
	thinkingMarkerFmt = "\n<!-- THINKING:%s -->\n"
	planStartMarker   = "\n<!-- PLAN_START -->\n"
	planEndMarker     = "\n<!-- PLAN_END -->\n"
)

var contentMarkerRegex = regexp.MustCompile(`\n?<!-- (?:TOOL_CALL:.+?|THINKING:.+?|PLAN_START|PLAN_END) -->\n?`)

// StripContentMarkers removes TOOL_CALL/PLAN markers from text for real-time display.
func StripContentMarkers(input string) string {
	return contentMarkerRegex.ReplaceAllString(input, "")
}

// chatTurnState holds intermediate state while preparing a chat turn.
type chatTurnState struct {
	session           *domain.Session
	modelConfig       llmsvc.ModelRuntimeConfig
	contextMessages   []llmsvc.ChatMessage
	enabledMCPConfigs []domain.MCPConfig
	attachments       []UploadedAttachment
}

// chatTurnResult holds intermediate results after the agent runs.
type chatTurnResult struct {
	answer        string
	interrupted   bool
	planCompleted bool
	title         string
	memoryPayload string
	pushErr       error
	narration     string
	planBody      string
}

// HandleChatStream runs one full turn: persist user message, build context, run agent, save assistant.
func (s *ChatService) HandleChatStream(
	ctx context.Context,
	sessionID string,
	requestID string,
	content string,
	displayContent string,
	modelID string,
	attachmentIDs []string,
	thinkingLevel string,
	planMode bool,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	if strings.TrimSpace(content) == "" && len(attachmentIDs) == 0 {
		return nil, fmt.Errorf("Message cannot be empty.")
	}

	state, err := s.prepareChatTurn(ctx, sessionID, content, displayContent, modelID, attachmentIDs, thinkingLevel)
	if err != nil {
		return nil, err
	}
	defer s.cleanupTurnAttachments(state.attachments)

	if planMode {
		state.contextMessages = append(state.contextMessages, llmsvc.ChatMessage{
			Role:    "system",
			Content: planModeSystemMessage,
		})
	}

	result, err := s.executeChatTurn(ctx, sessionID, requestID, state, callbacks, planMode)
	if err != nil {
		return nil, err
	}

	return s.finalizeChatTurn(ctx, sessionID, requestID, state, result, planMode)
}

// prepareChatTurn validates input, resolves model config, saves user message, builds context in parallel.
func (s *ChatService) prepareChatTurn(
	ctx context.Context,
	sessionID string,
	content string,
	displayContent string,
	modelID string,
	attachmentIDs []string,
	thinkingLevel string,
) (*chatTurnState, error) {
	llmConfig, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return nil, err
	}
	modelConfig := llmsvc.ModelRuntimeConfig{
		Provider:      llmConfig.Provider,
		BaseURL:       llmConfig.BaseURL,
		APIKey:        llmConfig.APIKey,
		Model:         llmConfig.Model,
		ThinkingLevel: thinkingLevel,
	}

	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("Session not found: %s.", sessionID)
		}
		return nil, err
	}

	attachments, err := s.resolveTurnAttachments(sessionID, attachmentIDs)
	if err != nil {
		return nil, err
	}

	userContentForLLM := strings.TrimSpace(content)
	userContentForDisplay := content
	if strings.TrimSpace(displayContent) != "" {
		userContentForDisplay = displayContent
	}
	userMessageParts := make([]llmsvc.ChatMessageContentPart, 0)
	var attachmentFallback []string
	if len(attachments) > 0 {
		userMessageParts, attachmentFallback = buildUserMessageContentParts(userContentForLLM, attachments)
		if len(userMessageParts) == 0 || len(attachmentFallback) > 0 {
			userContentForLLM = buildUserPromptWithAttachments(userContentForLLM, attachments)
		}
	}

	userMessageAttachments := make([]domain.MessageAttachment, 0, len(attachments))
	for _, item := range attachments {
		userMessageAttachments = append(userMessageAttachments, item.ToMessageAttachment())
	}
	if _, err := s.store.AddMessageWithInput(ctx, domain.AddMessageInput{
		SessionID:   sessionID,
		Role:        "user",
		Content:     userContentForDisplay,
		Attachments: userMessageAttachments,
	}); err != nil {
		return nil, err
	}

	// Build context messages and enabled MCP configs in parallel to reduce turn latency.
	var (
		contextMessages   []llmsvc.ChatMessage
		enabledMCPConfigs []domain.MCPConfig
		contextErr        error
		mcpErr            error
	)
	var prepareWG sync.WaitGroup
	prepareWG.Add(2)
	go func() {
		defer prepareWG.Done()
		contextMessages, contextErr = s.BuildContextMessages(ctx, sessionID, modelConfig)
	}()
	go func() {
		defer prepareWG.Done()
		enabledMCPConfigs, mcpErr = s.store.ListEnabledMCPConfigs(ctx)
	}()
	prepareWG.Wait()
	if contextErr != nil {
		return nil, contextErr
	}
	if mcpErr != nil {
		return nil, mcpErr
	}

	if len(attachments) > 0 {
		if len(userMessageParts) > 0 {
			overrideLatestUserTurnWithParts(contextMessages, userContentForLLM, userMessageParts)
		} else {
			overrideLatestUserTurn(contextMessages, userContentForLLM)
		}
	} else if strings.TrimSpace(displayContent) != "" {
		overrideLatestUserTurn(contextMessages, userContentForLLM)
	}
	appendProtocolHintToLatestUser(contextMessages, time.Now())

	return &chatTurnState{
		session:           session,
		modelConfig:       modelConfig,
		contextMessages:   contextMessages,
		enabledMCPConfigs: enabledMCPConfigs,
		attachments:       attachments,
	}, nil
}

// executeChatTurn runs the agent loop and collects streamed output.
func (s *ChatService) executeChatTurn(
	ctx context.Context,
	sessionID string,
	requestID string,
	state *chatTurnState,
	callbacks AgentCallbacks,
	planMode bool,
) (*chatTurnResult, error) {
	parser := newTitleStreamParser(!state.session.IsTitleLocked)
	accumulator := &chatStreamAccumulator{}
	streamStart := time.Now()
	var firstTokenAt time.Time
	var activeThinkingID string
	var activeThinkingDone bool

	pushBody := func(body string) error {
		if body == "" {
			return nil
		}
		accumulator.answerBuilder.WriteString(body)
		if accumulator.pushErr != nil {
			return nil
		}
		if planMode {
			if accumulator.planStarted {
				accumulator.planBodyBuilder.WriteString(body)
				return nil // plan body: buffer only, send via OnPlanBody
			}
			accumulator.narrationBuilder.WriteString(body)
			// Narration: buffer AND stream in real-time (fall through to OnChunk)
		}
		if err := callbacks.OnChunk(body); err != nil {
			accumulator.pushErr = err
		}
		return nil
	}
	finishActiveThinking := func(finishedAt time.Time) error {
		if activeThinkingID == "" || activeThinkingDone {
			return nil
		}
		if err := s.store.FinishThinking(ctx, domain.ThinkingFinishRecordInput{
			SessionID:  sessionID,
			RequestID:  requestID,
			ThinkingID: activeThinkingID,
			FinishedAt: finishedAt,
		}); err != nil {
			return err
		}
		activeThinkingDone = true
		return nil
	}

	agentCallbacks := AgentCallbacks{
		OnChunk: func(chunk string) error {
			if chunk != "" && firstTokenAt.IsZero() {
				firstTokenAt = time.Now()
			}
			return pushBody(parser.Feed(chunk))
		},
		OnToolCallStart: func(req ApprovalRequest) error {
			startStatus := constants.ToolCallStatusExecuting
			if req.RequiresApproval {
				startStatus = constants.ToolCallStatusPending
			}
			if err := s.recordToolCallStart(ctx, sessionID, requestID, req, startStatus); err != nil {
				return err
			}
			if err := pushBody(parser.BeginAssistantTurn()); err != nil {
				return err
			}
			// Insert marker into stored answer (not streamed to client).
			accumulator.answerBuilder.WriteString(fmt.Sprintf(toolCallMarkerFmt, req.ToolCallID))
			if callbacks.OnToolCallStart == nil {
				return nil
			}
			// In plan mode, narration text is streamed in real-time.
			// Clear preamble since text was already sent via chunks.
			if planMode {
				req.Preamble = ""
			}
			return callbacks.OnToolCallStart(req)
		},
		WaitApproval: callbacks.WaitApproval,
		OnToolCallResult: func(result ToolCallResult) error {
			status := normalizeToolCallResultStatus(result)
			if err := s.recordToolCallResult(ctx, sessionID, requestID, result, status); err != nil {
				return err
			}
			if callbacks.OnToolCallResult == nil {
				return nil
			}
			return callbacks.OnToolCallResult(result)
		},
		OnSubagentStart: func(parentToolCallID, runID, task string) error {
			if callbacks.OnSubagentStart != nil {
				return callbacks.OnSubagentStart(parentToolCallID, runID, task)
			}
			return nil
		},
		OnSubagentChunk: func(parentToolCallID, runID, chunk string) error {
			if callbacks.OnSubagentChunk != nil {
				return callbacks.OnSubagentChunk(parentToolCallID, runID, chunk)
			}
			return nil
		},
		OnSubagentDone: func(parentToolCallID, runID string, runErr error) error {
			if callbacks.OnSubagentDone != nil {
				return callbacks.OnSubagentDone(parentToolCallID, runID, runErr)
			}
			return nil
		},
		OnThinkingStart: func() error {
			if err := finishActiveThinking(time.Now()); err != nil {
				return err
			}
			activeThinkingID = uuid.NewString()
			activeThinkingDone = false
			if err := s.store.UpsertThinkingStart(ctx, domain.ThinkingStartRecordInput{
				SessionID:  sessionID,
				RequestID:  requestID,
				ThinkingID: activeThinkingID,
				StartedAt:  time.Now(),
			}); err != nil {
				return err
			}
			accumulator.answerBuilder.WriteString(fmt.Sprintf(thinkingMarkerFmt, activeThinkingID))
			if callbacks.OnThinkingStart == nil {
				return nil
			}
			return callbacks.OnThinkingStart()
		},
		OnThinkingChunk: func(chunk string) error {
			if activeThinkingID != "" {
				if err := s.store.AppendThinkingChunk(ctx, domain.ThinkingChunkRecordInput{
					SessionID:  sessionID,
					RequestID:  requestID,
					ThinkingID: activeThinkingID,
					Chunk:      chunk,
				}); err != nil {
					return err
				}
			}
			if callbacks.OnThinkingChunk == nil {
				return nil
			}
			return callbacks.OnThinkingChunk(chunk)
		},
		OnThinkingDone: func() error {
			if err := finishActiveThinking(time.Now()); err != nil {
				return err
			}
			if callbacks.OnThinkingDone == nil {
				return nil
			}
			return callbacks.OnThinkingDone()
		},
		OnPlanStart: func() error {
			accumulator.planStarted = true
			accumulator.answerBuilder.WriteString(planStartMarker)
			return nil
		},
	}

	activatedSkills := s.getSessionActivatedSkills(sessionID)

	approvalMode := constants.ApprovalModeStandard
	if s.settingsStore != nil {
		if mode, err := s.settingsStore.GetSetting(ctx, constants.SettingApprovalMode); err == nil && mode != "" {
			approvalMode = mode
		}
	}

	agentStart := time.Now()
	var planCompleted bool
	answer, err := s.agent.RunAgentLoop(ctx, state.modelConfig, sessionID, state.contextMessages, state.enabledMCPConfigs, activatedSkills, agentCallbacks, AgentLoopOptions{ApprovalMode: approvalMode, PlanMode: planMode, PlanComplete: &planCompleted})
	logging.Span("agent_loop", agentStart)
	s.mergeSessionActivatedSkills(sessionID, activatedSkills)
	if finishErr := finishActiveThinking(time.Now()); finishErr != nil && err == nil {
		err = finishErr
	}

	interrupted := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	if err != nil && !interrupted && answer == "" {
		return nil, err
	}

	firstTokenMs := int64(-1)
	if !firstTokenAt.IsZero() {
		firstTokenMs = firstTokenAt.Sub(streamStart).Milliseconds()
	}
	logging.Info("chat_stream_done", "session", sessionID, "first_token_ms", firstTokenMs, "total_stream_ms", time.Since(streamStart).Milliseconds())

	if err := pushBody(parser.Flush()); err != nil && !interrupted {
		return nil, err
	}

	var finalAnswer string
	var resultNarration string
	var resultPlanBody string

	if planMode {
		if accumulator.planStarted {
			accumulator.answerBuilder.WriteString(planEndMarker)
		}
		accumulated := strings.TrimSpace(accumulator.answerBuilder.String())
		finalAnswer = accumulated

		var narration, planBody string
		if accumulator.planStarted {
			narration = strings.TrimSpace(accumulator.narrationBuilder.String())
			planBody = accumulator.planBodyBuilder.String()
		} else {
			// Fallback: model did not call plan_start, use heuristic split.
			narration, planBody = splitNarrationAndPlan(accumulated)
		}
		resultNarration = narration
		resultPlanBody = planBody

		// Send plan body via OnPlanBody (non-streaming)
		if planBody != "" && callbacks.OnPlanBody != nil {
			if sendErr := callbacks.OnPlanBody(planBody); sendErr != nil && !interrupted {
				return nil, sendErr
			}
		}
	} else {
		finalAnswer = strings.TrimSpace(accumulator.answerBuilder.String())
		if finalAnswer == "" {
			finalAnswer = answer
		}
	}

	title := parser.Title()
	memoryPayload := parser.Memory()
	if parsedTitle, parsedMemory, cleanBody := extractProtocolMetaAndBody(finalAnswer); parsedTitle != "" || parsedMemory != "" || cleanBody != finalAnswer {
		if parsedTitle != "" {
			title = parsedTitle
		}
		if parsedMemory != "" {
			memoryPayload = parsedMemory
		}
		finalAnswer = cleanBody
	}
	if strings.TrimSpace(finalAnswer) == "" && !interrupted {
		finalAnswer = "The model returned no content."
	}

	return &chatTurnResult{
		answer:        finalAnswer,
		interrupted:   interrupted,
		planCompleted: planCompleted,
		title:         title,
		memoryPayload: memoryPayload,
		pushErr:       accumulator.pushErr,
		narration:     resultNarration,
		planBody:      resultPlanBody,
	}, nil
}

// finalizeChatTurn persists assistant message, updates title, enqueues memory, saves plan if applicable.
func (s *ChatService) finalizeChatTurn(
	ctx context.Context,
	sessionID string,
	requestID string,
	state *chatTurnState,
	result *chatTurnResult,
	planMode bool,
) (*ChatStreamResult, error) {
	assistantMessage, err := s.store.AddMessageWithInput(ctx, domain.AddMessageInput{
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           result.answer,
		IsInterrupted:     result.interrupted,
		IsStopPlaceholder: result.interrupted && strings.TrimSpace(result.answer) == "",
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.BindToolCallsToAssistantMessage(ctx, sessionID, requestID, assistantMessage.ID); err != nil {
		return nil, err
	}
	if err := s.store.BindThinkingRecordsToAssistantMessage(ctx, sessionID, requestID, assistantMessage.ID); err != nil {
		return nil, err
	}

	streamResult := &ChatStreamResult{
		Answer:            result.answer,
		IsInterrupted:     result.interrupted,
		IsStopPlaceholder: result.interrupted && strings.TrimSpace(result.answer) == "",
		Narration:         result.narration,
		PlanBody:          result.planBody,
	}
	if err := s.applySessionTitleUpdate(ctx, s.store, state.session, result.title, streamResult); err != nil {
		return nil, err
	}
	if s.memory != nil && strings.TrimSpace(result.memoryPayload) != "" {
		s.memory.EnqueueTurnMemory(sessionID, assistantMessage.ID, result.memoryPayload)
		logging.Info("memory_enqueue_triggered", "session", sessionID)
	} else if s.memory != nil {
		logging.Info("memory_enqueue_skipped", "session", sessionID, "reason", "empty_or_unparsed")
	}

	if planMode && result.planCompleted && s.planService != nil && strings.TrimSpace(result.answer) != "" {
		title := "Plan"
		for _, line := range strings.Split(result.answer, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				break
			}
		}
		if plan, saveErr := s.planService.SavePlan(sessionID, title, result.answer); saveErr != nil {
			logging.Info("plan_save_error", "session", sessionID, "error", saveErr.Error())
		} else {
			logging.Info("plan_saved", "session", sessionID)
			streamResult.PlanID = plan.ID
		}
	}

	if result.pushErr != nil {
		streamResult.PushFailed = true
		streamResult.PushError = result.pushErr.Error()
	}
	return streamResult, nil
}

type sessionTitleUpdater interface {
	UpdateSessionTitle(ctx context.Context, id, name string) (bool, error)
}

func (s *ChatService) applySessionTitleUpdate(ctx context.Context, store sessionTitleUpdater, session *domain.Session, title string, result *ChatStreamResult) error {
	_ = s
	if store == nil || session == nil || result == nil {
		return nil
	}
	title = strings.TrimSpace(title)
	if title == "" || session.IsTitleLocked {
		return nil
	}
	if session.Name != "" && session.Name != "New Chat" && session.Name == title {
		return nil
	}
	updated, err := store.UpdateSessionTitle(ctx, session.ID, title)
	if err != nil {
		return err
	}
	if updated {
		result.TitleUpdated = true
		result.Title = title
	}
	return nil
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

1. **Research** — Use read-only tools (web_search, search_memory) ONLY to gather information.
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
- Only web_search, search_memory, plan_start, and plan_complete__submit tools are available.`
