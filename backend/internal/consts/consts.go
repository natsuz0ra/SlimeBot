package consts

import "time"

const (
	// Chat context limits.
	ContextHistoryLimit = 20
	TitleProbeRuneLimit = 100
)

const (
	// Fixed session for message platforms.
	MessagePlatformSessionID   = "im-platform-session"
	MessagePlatformSessionName = "消息平台会话"
)

const (
	// Shared settings keys.
	SettingLanguage                    = "language"
	SettingDefaultModel                = "defaultModel"
	SettingMessagePlatformDefaultModel = "messagePlatformDefaultModel"
)

const (
	// Tool call statuses.
	ToolCallStatusPending   = "pending"
	ToolCallStatusExecuting = "executing"
	ToolCallStatusCompleted = "completed"
	ToolCallStatusError     = "error"
	ToolCallStatusRejected  = "rejected"
)

const (
	// Agent limits and defaults.
	AgentMaxIterations    = 50
	AgentApprovalTimeout  = 120 * time.Second
	MaxToolNameLen        = 64
	MemoryToolDefaultTopK = 3
)

const (
	// Stream result enum values.
	StreamResultText      = 0
	StreamResultToolCalls = 1
)

const (
	// Memory processing controls.
	CompressHistoryThreshold       = 10
	CompactRawHistoryLimit         = 6
	MemorySearchTopK               = 5
	MemoryDecisionTimeout          = 20 * time.Second
	MemorySummaryTimeout           = 45 * time.Second
	MemoryCallMaxAttempts          = 2
	MemoryRetryBackoff             = 350 * time.Millisecond
	MemorySummaryRecentMessageSize = 20
	MemoryKeywordMaxCount          = 12
)

const (
	// Skill package limits.
	MaxSkillZipBytes       = 20 * 1024 * 1024
	MaxSkillExtractedBytes = 50 * 1024 * 1024
	MaxSkillSingleFileSize = 10 * 1024 * 1024
	MaxSkillFileCount      = 2000
	MaxSkillResourcesShown = 200
)

const (
	// Auth settings and context keys.
	SettingAuthUsername            = "auth.username"
	SettingAuthPasswordHash        = "auth.password_hash"
	SettingAuthForcePasswordChange = "auth.force_password_change"
	ContextAuthUsername            = "auth.username"
)

const (
	// Telegram worker constants.
	TelegramPollTimeoutSeconds = 25
	TelegramIdleWaitInterval   = 60 * time.Second
	TelegramErrorBackoff       = 5 * time.Second
	TelegramPlatformName       = "telegram"
)

const (
	// Repository memory search bounds.
	DefaultMemoryCandidateLimit = 200
	MaxMemoryCandidateLimit     = 1000
)

const (
	// exec tool limits.
	ExecDefaultTimeout  = 30
	ExecMaxTimeout      = 300
	ExecMaxOutputBytes  = 64 * 1024
	ExecToolName        = "exec"
	ActivateSkillTool   = "activate_skill"
	MemoryQueryToolName = "memory__query"
)

const (
	// HTTP request tool defaults.
	HTTPRequestTimeout   = 30 * time.Second
	HTTPMaxResponseBytes = 128 * 1024
)

const (
	// Web search tool defaults.
	WebSearchBaseURL         = "https://api.tavily.com"
	WebSearchTimeout         = 20 * time.Second
	WebSearchMaxResponseSize = 256 * 1024
	WebSearchMaxSources      = 5
	WebSearchMaxContentRunes = 300
)

const (
	// MCP function name limits.
	MCPFuncNameMaxLen = 64
	MCPFuncHashLen    = 8
)
