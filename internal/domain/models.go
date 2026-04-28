package domain

import "time"

type Session struct {
	ID            string     `gorm:"primaryKey;size:36" json:"id"`
	Name          string     `gorm:"size:128;not null" json:"name"`
	IsTitleLocked bool       `gorm:"default:false;not null" json:"isTitleLocked"`
	ModelConfigID *string    `gorm:"size:36" json:"modelConfigId,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"index" json:"updatedAt"`
	DeletedAt     *time.Time `gorm:"index" json:"-"`
}

type Message struct {
	ID        string `gorm:"primaryKey;size:36" json:"id"`
	SessionID string `gorm:"size:36;index;index:idx_messages_session_created,priority:1;not null" json:"sessionId"`
	Role      string `gorm:"size:16;index;not null" json:"role"`
	Content   string `gorm:"type:text;not null" json:"content"`
	// IsInterrupted is true if streaming assistant output was cancelled by the user or context.
	IsInterrupted bool `gorm:"not null;default:false" json:"isInterrupted"`
	// IsStopPlaceholder covers interrupt-with-no-body; UI can show i18n placeholder text.
	IsStopPlaceholder bool `gorm:"not null;default:false" json:"isStopPlaceholder"`
	// AttachmentsJSON persists attachment metadata JSON (not file bytes).
	AttachmentsJSON string `gorm:"type:text;not null;default:'[]'" json:"-"`
	// Attachments is the runtime slice exposed to the frontend for attachment cards.
	Attachments []MessageAttachment `gorm:"-" json:"attachments"`
	CreatedAt   time.Time           `gorm:"index;index:idx_messages_session_created,priority:2" json:"createdAt"`
	Seq         int64               `gorm:"not null;default:0;index:idx_messages_session_created,priority:3" json:"seq"`
}

// MessageAttachment is attachment metadata for a message (no raw file bytes).
type MessageAttachment struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Ext       string `json:"ext"`
	SizeBytes int64  `json:"sizeBytes"`
	MimeType  string `json:"mimeType"`
	Category  string `json:"category,omitempty"`
	IconType  string `json:"iconType"`
}

// ToolCallRecord persists one tool call lifecycle for history replay.
type ToolCallRecord struct {
	ID                 string     `gorm:"primaryKey;size:36" json:"id"`
	SessionID          string     `gorm:"size:36;index;not null;uniqueIndex:idx_tool_call_request,priority:1" json:"sessionId"`
	RequestID          string     `gorm:"size:36;index;not null;uniqueIndex:idx_tool_call_request,priority:2" json:"requestId"`
	AssistantMessageID *string    `gorm:"size:36;index" json:"assistantMessageId,omitempty"`
	ToolCallID         string     `gorm:"size:128;index;not null;uniqueIndex:idx_tool_call_request,priority:3" json:"toolCallId"`
	ToolName           string     `gorm:"size:128;not null" json:"toolName"`
	Command            string     `gorm:"size:128;not null" json:"command"`
	ParamsJSON         string     `gorm:"type:text;not null" json:"paramsJson"`
	Status             string     `gorm:"size:32;index;not null" json:"status"`
	RequiresApproval   bool       `gorm:"not null;default:false" json:"requiresApproval"`
	ParentToolCallID   string     `gorm:"size:128;index" json:"parentToolCallId,omitempty"`
	SubagentRunID      string     `gorm:"size:128;index" json:"subagentRunId,omitempty"`
	Output             string     `gorm:"type:text" json:"output,omitempty"`
	Error              string     `gorm:"type:text" json:"error,omitempty"`
	StartedAt          time.Time  `gorm:"index;not null" json:"startedAt"`
	FinishedAt         *time.Time `gorm:"index" json:"finishedAt,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// ThinkingRecord persists one model thinking block for history replay.
type ThinkingRecord struct {
	ID                 string     `gorm:"primaryKey;size:36" json:"id"`
	SessionID          string     `gorm:"size:36;index;not null;uniqueIndex:idx_thinking_request,priority:1" json:"sessionId"`
	RequestID          string     `gorm:"size:36;index;not null;uniqueIndex:idx_thinking_request,priority:2" json:"requestId"`
	AssistantMessageID *string    `gorm:"size:36;index" json:"assistantMessageId,omitempty"`
	ThinkingID         string     `gorm:"size:128;index;not null;uniqueIndex:idx_thinking_request,priority:3" json:"thinkingId"`
	ParentToolCallID   string     `gorm:"size:128;index" json:"parentToolCallId,omitempty"`
	SubagentRunID      string     `gorm:"size:128;index" json:"subagentRunId,omitempty"`
	Content            string     `gorm:"type:text" json:"content"`
	Status             string     `gorm:"size:32;index;not null" json:"status"`
	StartedAt          time.Time  `gorm:"index;not null" json:"startedAt"`
	FinishedAt         *time.Time `gorm:"index" json:"finishedAt,omitempty"`
	DurationMs         int64      `gorm:"not null;default:0" json:"durationMs"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type AppSetting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LLMConfig struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Provider  string    `gorm:"size:32;not null;default:'openai'" json:"provider"`
	BaseURL   string    `gorm:"size:512;not null" json:"baseUrl"`
	APIKey    string    `gorm:"size:512;not null" json:"apiKey"`
	Model     string    `gorm:"size:128;not null" json:"model"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type MCPConfig struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Config    string    `gorm:"type:text;not null" json:"config"`
	IsEnabled bool      `gorm:"default:true;not null" json:"isEnabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MessagePlatformConfig stores external messaging platform settings (Telegram first).
// AuthConfigJSON is a JSON object for extensible per-platform auth fields.
type MessagePlatformConfig struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	Platform       string    `gorm:"size:32;not null;uniqueIndex" json:"platform"`
	DisplayName    string    `gorm:"size:64;not null" json:"displayName"`
	AuthConfigJSON string    `gorm:"type:text;not null" json:"authConfigJson"`
	IsEnabled      bool      `gorm:"default:true;not null" json:"isEnabled"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Plan is a markdown implementation plan generated by the LLM in plan mode.
type Plan struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	SessionID  string `json:"sessionId"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	ApprovedAt string `json:"approvedAt,omitempty"`
	Content    string `json:"content"`
}

type Skill struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	RelativePath string    `json:"relativePath"`
	Description  string    `json:"description"`
	UploadedAt   time.Time `json:"uploadedAt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
