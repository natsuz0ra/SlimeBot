package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/observability"
	oaisvc "slimebot/internal/services/openai"
)

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

	llmConfig, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return nil, err
	}
	modelConfig := oaisvc.ModelRuntimeConfig{
		BaseURL: llmConfig.BaseURL,
		APIKey:  llmConfig.APIKey,
		Model:   llmConfig.Model,
	}

	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("Session not found: %s.", sessionID)
	}

	attachments, err := s.resolveTurnAttachments(sessionID, attachmentIDs)
	if err != nil {
		return nil, err
	}
	defer s.cleanupTurnAttachments(attachments)

	userContentForLLM := strings.TrimSpace(content)
	userMessageParts := make([]oaisvc.ChatMessageContentPart, 0)
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

	// 上下文消息与启用的 MCP 配置彼此独立，先并行准备以缩短回合启动耗时。
	var (
		contextMessages   []oaisvc.ChatMessage
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

	parser := newTitleStreamParser(!session.IsTitleLocked)
	accumulator := &chatStreamAccumulator{}
	streamStart := time.Now()
	var firstTokenAt time.Time

	// pushBody 负责同时维护最终答案缓存与对外流式推送，推送失败后不影响后续落库。
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
	answer, err := s.agent.RunAgentLoop(ctx, modelConfig, sessionID, contextMessages, enabledMCPConfigs, activatedSkills, agentCallbacks)
	observability.Span("agent_loop", agentStart)
	s.mergeSessionActivatedSkills(sessionID, activatedSkills)

	interrupted := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	if err != nil && !interrupted && answer == "" {
		return nil, err
	}

	firstTokenMs := int64(-1)
	if !firstTokenAt.IsZero() {
		firstTokenMs = firstTokenAt.Sub(streamStart).Milliseconds()
	}
	slog.Info("chat_stream_done", "session", sessionID, "first_token_ms", firstTokenMs, "total_stream_ms", time.Since(streamStart).Milliseconds())

	if err := pushBody(parser.Flush()); err != nil && !interrupted {
		return nil, err
	}

	finalAnswer := answer
	if strings.TrimSpace(finalAnswer) == "" {
		finalAnswer = strings.TrimSpace(accumulator.answerBuilder.String())
	}

	title := parser.Title()
	summary := parser.Summary()
	if parsedTitle, parsedSummary, cleanBody := extractProtocolMetaAndBody(finalAnswer); parsedTitle != "" || parsedSummary != "" || cleanBody != finalAnswer {
		if parsedTitle != "" {
			title = parsedTitle
		}
		if parsedSummary != "" {
			summary = parsedSummary
		}
		finalAnswer = cleanBody
	}
	if strings.TrimSpace(finalAnswer) == "" && !interrupted {
		finalAnswer = "The model returned no content."
	}

	assistantMessage, err := s.store.AddMessageWithInput(ctx, domain.AddMessageInput{
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           finalAnswer,
		IsInterrupted:     interrupted,
		IsStopPlaceholder: interrupted && strings.TrimSpace(finalAnswer) == "",
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.BindToolCallsToAssistantMessage(ctx, sessionID, requestID, assistantMessage.ID); err != nil {
		return nil, err
	}

	result := &ChatStreamResult{
		Answer:            finalAnswer,
		IsInterrupted:     interrupted,
		IsStopPlaceholder: interrupted && strings.TrimSpace(finalAnswer) == "",
	}
	if err := s.applySessionTitleUpdate(ctx, s.store, session, title, result); err != nil {
		return nil, err
	}
	if s.memory != nil && strings.TrimSpace(summary) != "" {
		s.memory.UpdateSummaryAsync(sessionID, summary)
		slog.Info("memory_summary_async_triggered", "session", sessionID)
	} else if s.memory != nil {
		slog.Info("memory_summary_skipped", "session", sessionID, "reason", "empty_or_unparsed")
	}
	if accumulator.pushErr != nil {
		result.PushFailed = true
		result.PushError = accumulator.pushErr.Error()
	}
	return result, nil
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
