package chat

import (
	"context"
	llmsvc "slimebot/internal/services/llm"
)

// ApprovalRequest is sent to the client for tool-call approval.
type ApprovalRequest struct {
	ToolCallID       string         `json:"toolCallId"`
	ToolName         string         `json:"toolName"`
	Command          string         `json:"command"`
	Params           map[string]any `json:"params"`
	RequiresApproval bool           `json:"requiresApproval"`
	Preamble         string         `json:"preamble,omitempty"`
	ParentToolCallID string         `json:"parentToolCallId,omitempty"`
	SubagentRunID    string         `json:"subagentRunId,omitempty"`
}

// ApprovalResponse is the client's approval decision.
type ApprovalResponse struct {
	ToolCallID string `json:"toolCallId"`
	Approved   bool   `json:"approved"`
	Answers    string `json:"answers,omitempty"` // JSON-encoded answers for ask_questions tool
}

// ToolCallResult is pushed to the client after tool execution.
type ToolCallResult struct {
	ToolCallID       string `json:"toolCallId"`
	ToolName         string `json:"toolName"`
	Command          string `json:"command"`
	RequiresApproval bool   `json:"requiresApproval"`
	Status           string `json:"status"`
	Output           string `json:"output"`
	Error            string `json:"error"`
	Metadata         any    `json:"metadata,omitempty"`
	ParentToolCallID string `json:"parentToolCallId,omitempty"`
	SubagentRunID    string `json:"subagentRunId,omitempty"`
}

type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"`
}

type TodoUpdate struct {
	Items []TodoItem `json:"items"`
	Note  string     `json:"note,omitempty"`
}

type ThinkingEventMeta struct {
	ParentToolCallID string
	SubagentRunID    string
}

type ContextUsage struct {
	SessionID        string `json:"sessionId"`
	ModelConfigID    string `json:"modelConfigId"`
	UsedTokens       int    `json:"usedTokens"`
	TotalTokens      int    `json:"totalTokens"`
	UsedPercent      int    `json:"usedPercent"`
	AvailablePercent int    `json:"availablePercent"`
	IsCompacted      bool   `json:"isCompacted"`
	CompactedAt      string `json:"compactedAt,omitempty"`
}

// AgentCallbacks wires the agent loop to the outside world (streaming, approval, results).
type AgentCallbacks struct {
	OnChunk            func(chunk string) error
	OnContextUsage     func(usage ContextUsage) error
	OnContextCompacted func(usage ContextUsage) error
	OnToolCallStart    func(req ApprovalRequest) error
	WaitApproval       func(ctx context.Context, toolCallID string) (*ApprovalResponse, error)
	OnToolCallResult   func(result ToolCallResult) error
	OnSubagentStart    func(parentToolCallID, runID, title, task string) error
	OnSubagentChunk    func(parentToolCallID, runID, chunk string) error
	OnSubagentDone     func(parentToolCallID, runID string, runErr error) error
	OnThinkingStart    func(meta ThinkingEventMeta) error
	OnThinkingChunk    func(chunk string, meta ThinkingEventMeta) error
	OnThinkingDone     func(meta ThinkingEventMeta) error
	OnTodoUpdate       func(update TodoUpdate) error
	OnPlanStart        func() error
	OnPlanChunk        func(chunk string) error
	OnPlanBody         func(planBody string) error
	OnTitleGenerated   func(sessionID, title string)
}

// AgentLoopOptions configures nested agent execution.
type AgentLoopOptions struct {
	Depth           int
	ApprovalMode    string
	PlanMode        bool
	PlanStarted     *bool
	PlanComplete    *bool
	SubagentModelID string
	LatestUsage     *llmsvc.TokenUsage
	OnProviderUsage func(usage llmsvc.TokenUsage) error
}
