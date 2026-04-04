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
	// IsInterrupted 标记 assistant 输出是否在流式阶段被用户主动中断或上下文取消。
	IsInterrupted bool `gorm:"not null;default:false" json:"isInterrupted"`
	// IsStopPlaceholder 用于“中断且无正文”场景，前端可据此展示 i18n 文案。
	IsStopPlaceholder bool `gorm:"not null;default:false" json:"isStopPlaceholder"`
	// AttachmentsJSON 为持久化字段，存储附件元信息数组（不含源文件内容）。
	AttachmentsJSON string `gorm:"type:text;not null;default:'[]'" json:"-"`
	// Attachments 为运行时反序列化字段，对外返回给前端渲染附件卡片。
	Attachments []MessageAttachment `gorm:"-" json:"attachments"`
	CreatedAt   time.Time           `gorm:"index;index:idx_messages_session_created,priority:2" json:"createdAt"`
	Seq         int64               `gorm:"not null;default:0;index:idx_messages_session_created,priority:3" json:"seq"`
}

// MessageAttachment 描述消息中的附件元信息（不包含源文件内容）
type MessageAttachment struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Ext       string `json:"ext"`
	SizeBytes int64  `json:"sizeBytes"`
	MimeType  string `json:"mimeType"`
	Category  string `json:"category,omitempty"`
	IconType  string `json:"iconType"`
}

type EpisodeMemory struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	SessionID      string    `gorm:"size:36;not null;index:idx_episode_memories_session_state,priority:1;index:idx_episode_memories_topic,priority:1" json:"sessionId"`
	TopicKey       string    `gorm:"size:128;not null;index:idx_episode_memories_topic,priority:2" json:"topicKey"`
	Title          string    `gorm:"size:128;not null" json:"title"`
	Summary        string    `gorm:"type:text;not null" json:"summary"`
	KeywordsJSON   string    `gorm:"type:text;not null;default:'[]'" json:"-"`
	State          string    `gorm:"size:24;not null;index:idx_episode_memories_session_state,priority:2" json:"state"`
	SourceStartSeq int64     `gorm:"not null;default:0" json:"sourceStartSeq"`
	SourceEndSeq   int64     `gorm:"not null;default:0" json:"sourceEndSeq"`
	TurnCount      int       `gorm:"not null;default:0" json:"turnCount"`
	LastActiveAt   time.Time `gorm:"index;not null" json:"lastActiveAt"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `gorm:"index" json:"updatedAt"`
}

type StickyMemory struct {
	ID             string     `gorm:"primaryKey;size:36" json:"id"`
	SessionID      string     `gorm:"size:36;not null;index:idx_sticky_memories_lookup,priority:1;index:idx_sticky_memories_prompt,priority:1" json:"sessionId"`
	Kind           string     `gorm:"size:32;not null;index:idx_sticky_memories_lookup,priority:2;index:idx_sticky_memories_prompt,priority:2" json:"kind"`
	Key            string     `gorm:"size:128;not null;index:idx_sticky_memories_lookup,priority:3" json:"key"`
	Value          string     `gorm:"type:text;not null" json:"value"`
	Summary        string     `gorm:"type:text;not null" json:"summary"`
	Confidence     float64    `gorm:"not null;default:0" json:"confidence"`
	Status         string     `gorm:"size:24;not null;index:idx_sticky_memories_prompt,priority:3" json:"status"`
	SourceStartSeq int64      `gorm:"not null;default:0" json:"sourceStartSeq"`
	SourceEndSeq   int64      `gorm:"not null;default:0" json:"sourceEndSeq"`
	LastSeenAt     time.Time  `gorm:"index;not null" json:"lastSeenAt"`
	ExpiresAt      *time.Time `gorm:"index" json:"expiresAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"index" json:"updatedAt"`
}

// ToolCallRecord 持久化一次工具调用完整链路，支持历史会话回放工具详情
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
	Output             string     `gorm:"type:text" json:"output,omitempty"`
	Error              string     `gorm:"type:text" json:"error,omitempty"`
	StartedAt          time.Time  `gorm:"index;not null" json:"startedAt"`
	FinishedAt         *time.Time `gorm:"index" json:"finishedAt,omitempty"`
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

// MessagePlatformConfig 保存外部消息平台接入配置（首期支持 Telegram）。
// AuthConfigJSON 使用 JSON 对象格式存储，便于后续扩展多平台多鉴权字段。
type MessagePlatformConfig struct {
	ID             string    `gorm:"primaryKey;size:36" json:"id"`
	Platform       string    `gorm:"size:32;not null;uniqueIndex" json:"platform"`
	DisplayName    string    `gorm:"size:64;not null" json:"displayName"`
	AuthConfigJSON string    `gorm:"type:text;not null" json:"authConfigJson"`
	IsEnabled      bool      `gorm:"default:true;not null" json:"isEnabled"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
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
