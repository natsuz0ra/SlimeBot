package constants

import "time"

const (
	ContextHistoryLimit                = 20
	MessagePlatformSessionID           = "im-platform-session"
	MessagePlatformSessionName         = "Message Platform Session"
	SettingLanguage                    = "language"
	SettingDefaultModel                = "defaultModel"
	SettingMessagePlatformDefaultModel = "messagePlatformDefaultModel"
	ToolCallStatusPending              = "pending"
	ToolCallStatusExecuting            = "executing"
	ToolCallStatusCompleted            = "completed"
	ToolCallStatusError                = "error"
	ToolCallStatusRejected             = "rejected"
	AgentMaxIterations                 = 50
	AgentApprovalTimeout               = 120 * time.Second
	MaxToolNameLen                     = 64
	MemoryToolDefaultTopK              = 3
	StreamResultText                   = 0
	StreamResultToolCalls              = 1
	CompressHistoryThreshold           = 10
	CompressedRecentHistoryLimit       = 10
	CompactRawHistoryLimit             = 6
	MemorySearchTopK                   = 5
	MemoryDecisionTimeout              = 20 * time.Second
	MemorySummaryTimeout               = 45 * time.Second
	MemoryCallMaxAttempts              = 2
	MemoryRetryBackoff                 = 350 * time.Millisecond
	MemorySummaryRecentMessageSize     = 20
	MemoryKeywordMaxCount              = 12
	MaxSkillZipBytes                   = 20 * 1024 * 1024
	MaxSkillExtractedBytes             = 50 * 1024 * 1024
	MaxSkillSingleFileSize             = 10 * 1024 * 1024
	MaxSkillFileCount                  = 2000
	MaxSkillResourcesShown             = 200
	SettingAuthUsername                = "auth.username"
	SettingAuthPasswordHash            = "auth.password_hash"
	SettingAuthForcePasswordChange     = "auth.force_password_change"
	ContextAuthUsername                = "auth.username"
	TelegramPollTimeoutSeconds         = 25
	TelegramIdleWaitInterval           = 60 * time.Second
	TelegramErrorBackoff               = 5 * time.Second
	TelegramPlatformName               = "telegram"
	DefaultMemoryCandidateLimit        = 200
	MaxMemoryCandidateLimit            = 1000
	ExecDefaultTimeout                 = 30
	ExecMaxTimeout                     = 300
	ExecMaxOutputBytes                 = 64 * 1024
	ExecToolName                       = "exec"
	ActivateSkillTool                  = "activate_skill"
	SearchMemoryTool                   = "search_memory"
	HTTPRequestTimeout                 = 30 * time.Second
	HTTPMaxResponseBytes               = 128 * 1024
	WebSearchBaseURL                   = "https://api.tavily.com"
	WebSearchTimeout                   = 20 * time.Second
	WebSearchMaxResponseSize           = 256 * 1024
	WebSearchMaxSources                = 5
	WebSearchMaxContentRunes           = 300
	MCPFuncNameMaxLen                  = 64
	MCPFuncHashLen                     = 8
)
