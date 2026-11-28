package domain

import (
	"context"

	llmsvc "slimebot/internal/services/llm"
)

type CompleteChatTurnInput struct {
	SessionID        string
	RequestID        string
	ContextRequestID string
	Content          string
	Interrupted      bool
	TokenUsage       *llmsvc.TokenUsage
}

type CompleteChatTurnResult struct {
	AssistantMessage *Message
	Deduplicated     bool
}

// ChatTurnCompleter atomically saves an assistant message and its turn records.
type ChatTurnCompleter interface {
	CompleteChatTurn(ctx context.Context, input CompleteChatTurnInput) (*CompleteChatTurnResult, error)
}

// ChatStore is the data access surface for the main chat flow.
type ChatStore interface {
	GetSessionByID(ctx context.Context, id string) (*Session, error)
	CreateSession(ctx context.Context, name string, workingDirectory ...string) (*Session, error)
	CreateSessionWithID(ctx context.Context, id, name string) (*Session, error)
	UpdateSessionTitle(ctx context.Context, id, name string) (bool, error)

	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	GetLLMConfigByID(ctx context.Context, id string) (*LLMConfig, error)
	ListLLMConfigs(ctx context.Context) ([]LLMConfig, error)

	ListRecentSessionMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	ListAllSessionMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	ListSessionToolCallRecordsByAssistantMessageIDs(ctx context.Context, sessionID string, messageIDs []string) ([]ToolCallRecord, error)
	GetSessionContextSummary(ctx context.Context, sessionID, modelConfigID string) (*SessionContextSummary, error)
	UpsertSessionContextSummary(ctx context.Context, item *SessionContextSummary) error

	ListEnabledMCPConfigs(ctx context.Context) ([]MCPConfig, error)

	AddMessageWithInput(ctx context.Context, input AddMessageInput) (*Message, error)
	UpdateUserMessageAndPruneAfter(ctx context.Context, sessionID, messageID, content string) (*Message, error)
	BindToolCallsToAssistantMessage(ctx context.Context, sessionID, requestID, assistantMessageID string) error
	BindThinkingRecordsToAssistantMessage(ctx context.Context, sessionID, requestID, assistantMessageID string) error
	FinishOpenToolCallsForRequest(ctx context.Context, sessionID, requestID, errorMessage string) error
	FinishOpenThinkingForRequest(ctx context.Context, sessionID, requestID string) error

	UpsertToolCallStart(ctx context.Context, input ToolCallStartRecordInput) error
	UpdateToolCallResult(ctx context.Context, input ToolCallResultRecordInput) error
	UpsertThinkingStart(ctx context.Context, input ThinkingStartRecordInput) error
	AppendThinkingChunk(ctx context.Context, input ThinkingChunkRecordInput) error
	FinishThinking(ctx context.Context, input ThinkingFinishRecordInput) error
}
