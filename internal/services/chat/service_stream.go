package chat

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
)

// chatTurnState 持有聊天回合准备阶段的中间状态。
type chatTurnState struct {
	session           *domain.Session
	modelConfig       llmsvc.ModelRuntimeConfig
	contextMessages   []llmsvc.ChatMessage
	enabledMCPConfigs []domain.MCPConfig
	attachments       []UploadedAttachment
}

// chatTurnResult 持有 Agent 执行后的中间结果。
type chatTurnResult struct {
	answer        string
	interrupted   bool
	title         string
	memoryPayload string
	pushErr       error
}

// HandleChatStream 执行一次完整聊天回合：写入用户消息、构建上下文、驱动 Agent、落库 assistant 结果。
func (s *ChatService) HandleChatStream(
	ctx context.Context,
	sessionID string,
	requestID string,
	content string,
	modelID string,
	attachmentIDs []string,
	callbacks AgentCallbacks,
) (*ChatStreamResult, error) {
	if strings.TrimSpace(content) == "" && len(attachmentIDs) == 0 {
		return nil, fmt.Errorf("Message cannot be empty.")
	}

	state, err := s.prepareChatTurn(ctx, sessionID, content, modelID, attachmentIDs)
	if err != nil {
		return nil, err
	}
	defer s.cleanupTurnAttachments(state.attachments)

	result, err := s.executeChatTurn(ctx, sessionID, requestID, state, callbacks)
	if err != nil {
		return nil, err
	}

	return s.finalizeChatTurn(ctx, sessionID, requestID, state, result)
}

// prepareChatTurn 验证输入、解析模型配置、写入用户消息、并行构建上下文。
func (s *ChatService) prepareChatTurn(
	ctx context.Context,
	sessionID string,
	content string,
	modelID string,
	attachmentIDs []string,
) (*chatTurnState, error) {
	llmConfig, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return nil, err
	}
	modelConfig := llmsvc.ModelRuntimeConfig{
		Provider: llmConfig.Provider,
		BaseURL:  llmConfig.BaseURL,
		APIKey:   llmConfig.APIKey,
		Model:    llmConfig.Model,
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
		Content:     content,
		Attachments: userMessageAttachments,
	}); err != nil {
		return nil, err
	}

	// 上下文消息与启用的 MCP 配置彼此独立，并行准备以缩短回合启动耗时。
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

// executeChatTurn 驱动 Agent 循环并收集流式结果。
func (s *ChatService) executeChatTurn(
	ctx context.Context,
	sessionID string,
	requestID string,
	state *chatTurnState,
	callbacks AgentCallbacks,
) (*chatTurnResult, error) {
	parser := newTitleStreamParser(!state.session.IsTitleLocked)
	accumulator := &chatStreamAccumulator{}
	streamStart := time.Now()
	var firstTokenAt time.Time

	pushBody := func(body string) error {
		if body == "" {
			return nil
		}
		accumulator.answerBuilder.WriteString(body)
		if accumulator.pushErr != nil {
			return nil
		}
		if err := callbacks.OnChunk(body); err != nil {
			accumulator.pushErr = err
		}
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
			if callbacks.OnToolCallStart == nil {
				return nil
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
	}

	activatedSkills := s.getSessionActivatedSkills(sessionID)
	agentStart := time.Now()
	answer, err := s.agent.RunAgentLoop(ctx, state.modelConfig, sessionID, state.contextMessages, state.enabledMCPConfigs, activatedSkills, agentCallbacks)
	logging.Span("agent_loop", agentStart)
	s.mergeSessionActivatedSkills(sessionID, activatedSkills)

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

	finalAnswer := answer
	if strings.TrimSpace(finalAnswer) == "" {
		finalAnswer = strings.TrimSpace(accumulator.answerBuilder.String())
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
		title:         title,
		memoryPayload: memoryPayload,
		pushErr:       accumulator.pushErr,
	}, nil
}

// finalizeChatTurn 落库 assistant 消息、更新标题、入队记忆。
func (s *ChatService) finalizeChatTurn(
	ctx context.Context,
	sessionID string,
	requestID string,
	state *chatTurnState,
	result *chatTurnResult,
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

	streamResult := &ChatStreamResult{
		Answer:            result.answer,
		IsInterrupted:     result.interrupted,
		IsStopPlaceholder: result.interrupted && strings.TrimSpace(result.answer) == "",
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
