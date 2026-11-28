package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	contextsvc "slimebot/internal/services/context"

	"github.com/google/uuid"
)

// TurnPersistenceError identifies a failed durable write during a chat turn.
type TurnPersistenceError struct {
	Operation string
	Err       error
}

func (e *TurnPersistenceError) Error() string {
	return fmt.Sprintf("persist chat turn %s: %v", e.Operation, e.Err)
}
func (e *TurnPersistenceError) Unwrap() error { return e.Err }

// TurnRecorder owns the durable tool, thinking and completion writes for one turn.
type TurnRecorder struct {
	store     domain.ChatStore
	sessionID string
	requestID string

	mu       sync.Mutex
	firstErr error

	thinkingMu sync.Mutex
	thinkings  map[thinkingScope]string
}

type thinkingScope struct {
	parentToolCallID string
	subagentRunID    string
}

func (r *TurnRecorder) track(operation string, err error) error {
	if err == nil {
		return nil
	}
	failure := &TurnPersistenceError{Operation: operation, Err: err}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.firstErr == nil {
		r.firstErr = failure
		logging.Warn("chat_turn_persistence_failed", "session", r.sessionID, "request", r.requestID, "operation", operation, "err", err)
	}
	return failure
}

func (r *TurnRecorder) failure() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.firstErr
}

func (r *TurnRecorder) write(ctx context.Context, operation string, save func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := save()
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return err
	}
	return r.track(operation, err)
}

func (r *TurnRecorder) startTool(ctx context.Context, req ApprovalRequest, status string) error {
	return r.write(ctx, "tool start", func() error {
		return r.store.UpsertToolCallStart(ctx, domain.ToolCallStartRecordInput{
			SessionID: r.sessionID, RequestID: r.requestID, ToolCallID: req.ToolCallID,
			ToolName: req.ToolName, Command: req.Command, ModelFuncName: req.ModelFuncName,
			Params: req.Params, Status: status, RequiresApproval: req.RequiresApproval,
			StartedAt: time.Now(), ParentToolCallID: req.ParentToolCallID, SubagentRunID: req.SubagentRunID,
		})
	})
}

func (r *TurnRecorder) finishTool(ctx context.Context, result ToolCallResult) error {
	return r.write(ctx, "tool result", func() error {
		return r.store.UpdateToolCallResult(ctx, domain.ToolCallResultRecordInput{
			SessionID: r.sessionID, RequestID: r.requestID, ToolCallID: result.ToolCallID,
			Status: normalizeToolCallResultStatus(result), Output: result.Output,
			Error: result.Error, Metadata: result.Metadata, FinishedAt: time.Now(),
		})
	})
}

func (r *TurnRecorder) finishOpenTools(ctx context.Context, reason string) error {
	return r.write(ctx, "finish open tools", func() error {
		return r.store.FinishOpenToolCallsForRequest(ctx, r.sessionID, r.requestID, reason)
	})
}

func (r *TurnRecorder) startThinking(ctx context.Context, meta ThinkingEventMeta) (string, error) {
	r.thinkingMu.Lock()
	defer r.thinkingMu.Unlock()
	now := time.Now()
	scope := thinkingScope{meta.ParentToolCallID, meta.SubagentRunID}
	if err := r.finishThinkingLocked(ctx, scope, now); err != nil {
		return "", err
	}
	id := uuid.NewString()
	if err := r.write(ctx, "thinking start", func() error {
		return r.store.UpsertThinkingStart(ctx, domain.ThinkingStartRecordInput{
			SessionID: r.sessionID, RequestID: r.requestID, ThinkingID: id,
			ParentToolCallID: meta.ParentToolCallID, SubagentRunID: meta.SubagentRunID, StartedAt: now,
		})
	}); err != nil {
		return "", err
	}
	r.thinkings[scope] = id
	return id, nil
}

func (r *TurnRecorder) appendThinking(ctx context.Context, meta ThinkingEventMeta, chunk string) error {
	r.thinkingMu.Lock()
	defer r.thinkingMu.Unlock()
	id := r.thinkings[thinkingScope{meta.ParentToolCallID, meta.SubagentRunID}]
	if id == "" {
		return nil
	}
	return r.write(ctx, "thinking chunk", func() error {
		return r.store.AppendThinkingChunk(ctx, domain.ThinkingChunkRecordInput{
			SessionID: r.sessionID, RequestID: r.requestID, ThinkingID: id, Chunk: chunk,
		})
	})
}

func (r *TurnRecorder) finishThinking(ctx context.Context, meta ThinkingEventMeta) error {
	r.thinkingMu.Lock()
	defer r.thinkingMu.Unlock()
	return r.finishThinkingLocked(ctx, thinkingScope{meta.ParentToolCallID, meta.SubagentRunID}, time.Now())
}

func (r *TurnRecorder) finishThinkings(ctx context.Context) error {
	r.thinkingMu.Lock()
	defer r.thinkingMu.Unlock()
	now := time.Now()
	for scope := range r.thinkings {
		if err := r.finishThinkingLocked(ctx, scope, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *TurnRecorder) finishThinkingLocked(ctx context.Context, scope thinkingScope, at time.Time) error {
	id := r.thinkings[scope]
	if id == "" {
		return nil
	}
	if err := r.write(ctx, "thinking finish", func() error {
		return r.store.FinishThinking(ctx, domain.ThinkingFinishRecordInput{
			SessionID: r.sessionID, RequestID: r.requestID, ThinkingID: id, FinishedAt: at,
		})
	}); err != nil {
		return err
	}
	delete(r.thinkings, scope)
	return nil
}

func toolStartStatus(req ApprovalRequest) string {
	if req.ReviewStatus == string(ApprovalReviewStatusReviewing) {
		return constants.ToolCallStatusReviewing
	}
	if req.RequiresApproval {
		return constants.ToolCallStatusPending
	}
	return constants.ToolCallStatusExecuting
}

func (r *TurnRecorder) complete(ctx context.Context, input domain.CompleteChatTurnInput) (*domain.CompleteChatTurnResult, error) {
	if store, ok := r.store.(domain.ChatTurnCompleter); ok {
		completed, err := store.CompleteChatTurn(ctx, input)
		return completed, r.track("completion", err)
	}
	assistant, err := r.completeWithChatStore(ctx, input)
	if err != nil {
		return nil, r.track("completion", err)
	}
	return &domain.CompleteChatTurnResult{AssistantMessage: assistant}, nil
}

// completeWithChatStore keeps support for stores that only implement ChatStore.
func (r *TurnRecorder) completeWithChatStore(ctx context.Context, input domain.CompleteChatTurnInput) (*domain.Message, error) {
	if input.Interrupted {
		if err := r.store.FinishOpenToolCallsForRequest(ctx, r.sessionID, r.requestID, "Execution cancelled."); err != nil {
			return nil, err
		}
		if err := r.store.FinishOpenThinkingForRequest(ctx, r.sessionID, r.requestID); err != nil {
			return nil, err
		}
	}
	assistant, err := r.store.AddMessageWithInput(ctx, domain.AddMessageInput{
		SessionID: r.sessionID, RequestID: r.requestID, Role: "assistant", Content: input.Content,
		IsInterrupted: input.Interrupted, IsStopPlaceholder: input.Interrupted && strings.TrimSpace(input.Content) == "",
		TokenUsage: input.TokenUsage,
	})
	if err != nil {
		return nil, err
	}
	if err := r.store.BindToolCallsToAssistantMessage(ctx, r.sessionID, r.requestID, assistant.ID); err != nil {
		return nil, err
	}
	if err := r.store.BindThinkingRecordsToAssistantMessage(ctx, r.sessionID, r.requestID, assistant.ID); err != nil {
		return nil, err
	}
	if store, ok := r.store.(contextsvc.Store); ok && input.ContextRequestID != "" {
		if err := store.BindContextRequest(ctx, r.sessionID, input.ContextRequestID, assistant.ID); err != nil {
			return nil, err
		}
	}
	return assistant, nil
}
