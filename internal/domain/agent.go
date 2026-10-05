package domain

import (
	"context"
	"errors"
	"time"
)

var ErrAgentConflict = errors.New("STALE_REVISION")
var ErrAgentCapacity = errors.New("CAPACITY_EXCEEDED")
var ErrAgentInboxFull = errors.New("INBOX_FULL")
var ErrAgentBudget = errors.New("BUDGET_EXCEEDED")

// Agent identities survive turns and process restarts. Secrets belong to model
// configuration storage, never to descriptors or events.
type AgentDescriptor struct {
	Queued        int       `gorm:"-" json:"queued"`
	SessionID     string    `gorm:"primaryKey;size:36" json:"id"`
	RootID        string    `gorm:"size:36;index;not null" json:"rootId"`
	ParentID      string    `gorm:"size:36;index;not null" json:"parentId"`
	RequestID     string    `gorm:"size:36;index" json:"requestId"`
	Title         string    `json:"title"`
	Task          string    `gorm:"type:text" json:"task"`
	Profile       string    `json:"profile"`
	Mode          string    `json:"mode"`
	ContextMode   string    `json:"contextMode"`
	ThinkingLevel string    `json:"-"`
	MaxDepth      int       `json:"maxDepth"`
	ModelID       string    `json:"modelId"`
	Workspace     string    `json:"workspace"`
	Depth         int       `json:"depth"`
	Version       int       `json:"version"`
	Archived      bool      `json:"archived"`
	PeerMessages  bool      `json:"peerMessages"`
	ApprovalMode  string    `json:"-"`
	PlanMode      bool      `json:"planMode"`
	TaskID        string    `json:"taskId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type AgentTurn struct {
	ID           string     `gorm:"primaryKey;size:36" json:"id"`
	SessionID    string     `gorm:"size:36;index" json:"agentId"`
	RootID       string     `gorm:"size:36;index" json:"rootId"`
	RequestID    string     `gorm:"size:36;index" json:"requestId"`
	MessageID    string     `gorm:"size:36;uniqueIndex" json:"messageId"`
	Status       string     `gorm:"index" json:"status"`
	StopReason   string     `json:"stopReason,omitempty"`
	Thinking     string     `gorm:"type:text" json:"-"`
	Answer       string     `gorm:"type:text" json:"answer,omitempty"`
	Error        string     `gorm:"type:text" json:"error,omitempty"`
	Revision     int64      `json:"revision"`
	Epoch        string     `json:"epoch"`
	InputTokens  int        `json:"inputTokens"`
	OutputTokens int        `json:"outputTokens"`
	Activity     string     `json:"activity,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

type AgentInbox struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	ClientID  string    `gorm:"uniqueIndex:idx_agent_inbox_client,priority:2" json:"clientId"`
	TargetID  string    `gorm:"size:36;index;uniqueIndex:idx_agent_inbox_client,priority:1" json:"targetId"`
	SenderID  string    `json:"senderId"`
	Source    string    `json:"source"`
	RequestID string    `gorm:"index" json:"requestId"`
	Content   string    `gorm:"type:text" json:"content"`
	Delivery  string    `json:"delivery"`
	Status    string    `gorm:"index" json:"status"`
	TaskID    string    `json:"taskId,omitempty"`
	TurnID    string    `json:"turnId,omitempty"`
	Seq       int64     `gorm:"index" json:"seq"`
	CreatedAt time.Time `json:"createdAt"`
}

type AgentRootRequest struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	SessionID string    `gorm:"size:36;index" json:"sessionId"`
	Status    string    `gorm:"index" json:"status"`
	Budget    int       `json:"budget"`
	Consumed  int       `json:"consumed"`
	Reserved  int       `json:"reserved"`
	Messages  int       `json:"messages"`
	CreatedAt time.Time `json:"createdAt"`
	Deadline  time.Time `json:"deadline"`
}

type AgentEvent struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"seq"`
	RootID    string    `gorm:"size:36;index" json:"rootId"`
	AgentID   string    `gorm:"size:36;index" json:"agentId"`
	TaskID    string    `json:"taskId,omitempty"`
	TurnID    string    `json:"turnId,omitempty"`
	Kind      string    `json:"kind"`
	Payload   string    `gorm:"type:text" json:"payload"`
	CreatedAt time.Time `json:"createdAt"`
}

type AgentTask struct {
	ID           string    `gorm:"primaryKey;size:36" json:"id"`
	RootID       string    `gorm:"size:36;index" json:"rootId"`
	RequestID    string    `json:"requestId"`
	Title        string    `json:"title"`
	Description  string    `gorm:"type:text" json:"description"`
	Acceptance   string    `json:"acceptance"`
	OwnerID      string    `json:"ownerId,omitempty"`
	Status       string    `gorm:"index" json:"status"`
	Revision     int64     `json:"revision"`
	Dependencies string    `gorm:"type:text" json:"dependencies"`
	WriteScopes  string    `gorm:"type:text" json:"writeScopes"`
	ArtifactID   string    `json:"artifactId,omitempty"`
	Result       string    `gorm:"type:text" json:"result,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type AgentArtifact struct {
	ID                string    `gorm:"primaryKey;size:36" json:"id"`
	RootID            string    `gorm:"size:36;index" json:"rootId"`
	AgentID           string    `gorm:"size:36;index" json:"agentId"`
	TaskID            string    `json:"taskId,omitempty"`
	WriteScopes       string    `gorm:"type:text" json:"writeScopes,omitempty"`
	GitDirectory      string    `json:"gitDirectory,omitempty"`
	ParentWorkspace   string    `json:"parentWorkspace"`
	Workspace         string    `json:"workspace"`
	BaseCommit        string    `json:"baseCommit"`
	ResultCommit      string    `json:"resultCommit,omitempty"`
	Status            string    `gorm:"index" json:"status"`
	Validation        string    `gorm:"type:text" json:"validation,omitempty"`
	ValidationCommand string    `gorm:"type:text" json:"validationCommand,omitempty"`
	ValidatedCommit   string    `json:"validatedCommit,omitempty"`
	Report            string    `gorm:"type:text" json:"report,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type AgentApproval struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	RootID     string    `gorm:"index" json:"rootId"`
	RequestID  string    `json:"requestId"`
	AgentID    string    `json:"agentId"`
	TurnID     string    `json:"turnId"`
	ToolCallID string    `json:"toolCallId"`
	Payload    string    `gorm:"type:text" json:"payload"`
	Status     string    `gorm:"index" json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
}

type AgentStore interface {
	CreateAgent(context.Context, *AgentDescriptor, *AgentInbox) error
	GetAgent(context.Context, string) (*AgentDescriptor, error)
	ListAgents(context.Context, string) ([]AgentDescriptor, error)
	AcceptAgentMessage(context.Context, *AgentInbox) error
	ClaimAgentMessage(context.Context, string, string, string) (*AgentInbox, *AgentTurn, error)
	ListAgentInbox(context.Context, string, string) ([]AgentInbox, error)
	ClaimAgentRelays(context.Context, string, string, bool) ([]AgentInbox, error)
	CompleteAgentTurn(context.Context, *AgentTurn, *AgentInbox) error
	UpdateAgentTurn(context.Context, *AgentTurn) error
	ListAgentTurns(context.Context, string) ([]AgentTurn, error)
	GetAgentTurn(context.Context, string, string) (*AgentTurn, error)
	PutAgentRoot(context.Context, *AgentRootRequest) error
	GetAgentRoot(context.Context, string) (*AgentRootRequest, error)
	UpdateAgentRoot(context.Context, string, string) error
	ReserveAgentBudget(context.Context, string, int) error
	SettleAgentBudget(context.Context, string, int, int) error
	AppendAgentEvent(context.Context, *AgentEvent) error
	ListAgentRoots(context.Context, string) ([]AgentRootRequest, error)
	ListAgentEvents(context.Context, string, int64) ([]AgentEvent, error)
	PutAgentTask(context.Context, *AgentTask, int64) error
	ListAgentTasks(context.Context, string) ([]AgentTask, error)
	PutAgentArtifact(context.Context, *AgentArtifact) error
	ListAgentArtifacts(context.Context, string) ([]AgentArtifact, error)
	RecoverAgentRuntime(context.Context) error
	GetAgentApproval(context.Context, string) (*AgentApproval, error)
	PutAgentApproval(context.Context, *AgentApproval) error
	ListAgentApprovals(context.Context, string) ([]AgentApproval, error)
	ResolveAgentApproval(context.Context, string, string) error
}
