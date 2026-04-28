package constants

import "time"

const (
	ContextHistoryLimit                = 10
	MessagePlatformSessionID           = "im-platform-session"
	MessagePlatformSessionName         = "Message Platform Session"
	SettingLanguage                    = "language"
	SettingDefaultModel                = "defaultModel"
	SettingMessagePlatformDefaultModel = "messagePlatformDefaultModel"
	SettingWebSearchAPIKey             = "WEB_SEARCH_API_KEY"
	ToolCallStatusPending              = "pending"
	ToolCallStatusExecuting            = "executing"
	ToolCallStatusCompleted            = "completed"
	ToolCallStatusError                = "error"
	ToolCallStatusRejected             = "rejected"
	AgentMaxIterations                 = 50
	// MaxSubagentDepth is max nesting: 0 = main only; 1 = one child level (child cannot run_subagent).
	MaxSubagentDepth               = 1
	AgentApprovalTimeout           = 120 * time.Second
	MaxToolNameLen                 = 64
	MemoryToolDefaultTopK          = 10
	CompressedRecentHistoryLimit   = 10
	MemorySearchTopK               = 5
	MemoryContextTopK              = 10
	MemoryContextBuildBudget       = 5 * time.Second
	MemoryKeywordMaxCount          = 12
	MaxSkillZipBytes               = 20 * 1024 * 1024
	MaxSkillExtractedBytes         = 50 * 1024 * 1024
	MaxSkillSingleFileSize         = 10 * 1024 * 1024
	MaxSkillFileCount              = 2000
	MaxSkillResourcesShown         = 200
	SettingAuthUsername            = "auth.username"
	SettingAuthPasswordHash        = "auth.password_hash"
	SettingAuthForcePasswordChange = "auth.force_password_change"
	ContextAuthUsername            = "auth.username"
	TelegramPollTimeoutSeconds     = 25
	TelegramIdleWaitInterval       = 60 * time.Second
	TelegramErrorBackoff           = 5 * time.Second
	TelegramPlatformName           = "telegram"
	ExecDefaultTimeoutMs           = 30_000
	ExecMaxTimeoutMs               = 600_000
	ExecMaxOutputBytes             = 64 * 1024
	ExecToolName                   = "exec"
	AskQuestionsTool               = "ask_questions"
	AskQuestionsMaxQuestions       = 5
	AskQuestionsMaxOptionsPerQ     = 5
	ActivateSkillTool              = "activate_skill"
	RunSubagentTool                = "run_subagent"
	SearchMemoryTool               = "search_memory"
	HTTPRequestTimeout             = 30 * time.Second
	HTTPMaxResponseBytes           = 128 * 1024
	WebSearchBaseURL               = "https://api.tavily.com"
	WebSearchTimeout               = 20 * time.Second
	WebSearchMaxResponseSize       = 256 * 1024
	WebSearchMaxSources            = 5
	WebSearchMaxContentRunes       = 300
	MCPFuncNameMaxLen              = 64
	MCPFuncHashLen                 = 8

	SettingApprovalMode  = "approvalMode"
	ApprovalModeStandard = "standard"
	ApprovalModeAuto     = "auto"

	SettingThinkingLevel = "thinkingLevel"

	// Plan mode
	PlanStartTool      = "plan_start"
	PlanCompleteTool   = "plan_complete__submit"
	PlanStatusPending  = "pending"
	PlanStatusApproved = "approved"
	PlanStatusRejected = "rejected"
	PlanStatusExecuted = "executed"

	// WebSocket
	WSChatTimeout     = 600 * time.Second
	WSWriteChannelBuf = 128
	WSChatChannelBuf  = 16
)
