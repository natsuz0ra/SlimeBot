package ws

import (
	"strings"
	"time"

	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"
	llmsvc "slimebot/internal/services/llm"
)

type wsSessionEvent struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
}

type wsSession struct{ wsSessionEvent }
type wsStopping struct{ wsSessionEvent }
type wsPlanStart struct{ wsSessionEvent }

type wsPong struct {
	Type string `json:"type"`
}

type wsError struct {
	Type      string  `json:"type"`
	SessionID *string `json:"sessionId,omitempty"`
	Error     string  `json:"error"`
}

type wsStart struct {
	wsSessionEvent
	TurnID    string `json:"turnId"`
	StartedAt string `json:"startedAt"`
}

type wsOutChunk struct {
	wsSessionEvent
	Content string `json:"content"`
}

type wsDone struct {
	wsSessionEvent
	FinishedAt        string                       `json:"finishedAt"`
	DurationMs        int64                        `json:"durationMs"`
	Answer            *string                      `json:"answer,omitempty"`
	IsInterrupted     *bool                        `json:"isInterrupted,omitempty"`
	IsStopPlaceholder *bool                        `json:"isStopPlaceholder,omitempty"`
	PlanID            string                       `json:"planId,omitempty"`
	PlanBody          *string                      `json:"planBody,omitempty"`
	Narration         *string                      `json:"narration,omitempty"`
	SkillLearning     *chatsvc.SkillLearningNotice `json:"skillLearning,omitempty"`
}

type wsMessageEdited struct {
	wsSessionEvent
	MessageID string `json:"messageId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}

type wsSessionTitle struct {
	wsSessionEvent
	Title string `json:"title"`
}

type wsThinkingStart struct {
	wsSessionEvent
	chatsvc.AgentEventMeta
	StartedAt string `json:"startedAt"`
}

type wsThinkingChunk struct {
	wsSessionEvent
	chatsvc.AgentEventMeta
	Content   string `json:"content"`
	StartedAt string `json:"startedAt"`
}

type wsThinkingDone struct {
	wsSessionEvent
	chatsvc.AgentEventMeta
	FinishedAt string `json:"finishedAt"`
}

type wsToolIdentity struct {
	chatsvc.AgentEventMeta
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Command    string `json:"command"`
}

type wsToolReview struct {
	wsToolIdentity
	ReviewStatus string `json:"reviewStatus"`
	ReviewRisk   string `json:"reviewRisk"`
	ReviewReason string `json:"reviewReason"`
}

type wsToolApproval struct {
	wsToolReview
	Params           map[string]any `json:"params"`
	RequiresApproval bool           `json:"requiresApproval"`
}

type wsToolCallStart struct {
	wsSessionEvent
	wsToolApproval
	Preamble  string `json:"preamble"`
	StartedAt string `json:"startedAt"`
}

type wsToolCallReview struct {
	wsSessionEvent
	wsToolReview
	ReviewedAt string `json:"reviewedAt"`
}

type wsToolApprovalRequired struct {
	wsSessionEvent
	wsToolApproval
	RequiredAt string `json:"requiredAt"`
}

type wsToolCallResult struct {
	wsSessionEvent
	wsToolIdentity
	RequiresApproval bool   `json:"requiresApproval"`
	Status           string `json:"status"`
	Output           string `json:"output"`
	Error            string `json:"error"`
	Metadata         any    `json:"metadata"`
	FinishedAt       string `json:"finishedAt"`
}

type wsTeamStart struct {
	wsSessionEvent
	RequestID   string `json:"requestId"`
	TeamRunID   string `json:"teamRunId"`
	Status      string `json:"status"`
	MaxMembers  int    `json:"maxMembers"`
	MaxParallel int    `json:"maxParallel"`
	StartedAt   string `json:"startedAt"`
}

type wsTeamDone struct {
	wsTeamStart
	LastError  string `json:"lastError"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

type wsTeamMemberQueued struct {
	wsSessionEvent
	TeamRunID     string `json:"teamRunId"`
	MemberRunID   string `json:"memberRunId"`
	ToolCallID    string `json:"toolCallId"`
	Title         string `json:"title"`
	Task          string `json:"task"`
	ModelConfigID string `json:"modelConfigId"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type wsSubagentIdentity struct {
	chatsvc.AgentEventMeta
	// Subagent lifecycle events always include these IDs, even when empty.
	ParentToolCallID string `json:"parentToolCallId"`
	SubagentRunID    string `json:"subagentRunId"`
}

type wsSubagentStart struct {
	wsSessionEvent
	wsSubagentIdentity
	Title string `json:"title"`
	Task  string `json:"task"`
}

type wsSubagentChunk struct {
	wsSessionEvent
	wsSubagentIdentity
	Content string `json:"content"`
}

type wsSubagentDone struct {
	wsSessionEvent
	wsSubagentIdentity
	Error *string `json:"error,omitempty"`
}

type wsPlanChunk struct {
	wsSessionEvent
	Content string `json:"content"`
}

type wsPlanBody struct {
	wsSessionEvent
	Content string `json:"content"`
}

type wsPlanStatus struct {
	Type   string `json:"type"`
	PlanID string `json:"planId"`
	Status string `json:"status"`
}

type wsTodoUpdate struct {
	wsSessionEvent
	Items     []chatsvc.TodoItem `json:"items"`
	Note      string             `json:"note,omitempty"`
	UpdatedAt string             `json:"updatedAt"`
}

// Flat usage events have historically included zero values and a nullable
// layers array. Compaction notifications use the nested service format.
type wsContextUsage struct {
	wsSessionEvent
	ModelConfigID          string                      `json:"modelConfigId"`
	UsedTokens             int                         `json:"usedTokens"`
	TotalTokens            int                         `json:"totalTokens"`
	UsedPercent            int                         `json:"usedPercent"`
	AvailablePercent       int                         `json:"availablePercent"`
	IsCompacted            bool                        `json:"isCompacted"`
	CompactedAt            string                      `json:"compactedAt"`
	InputBudget            int                         `json:"inputBudget"`
	OutputReserve          int                         `json:"outputReserve"`
	CompactionBeforeTokens int                         `json:"compactionBeforeTokens"`
	CompactionAfterTokens  int                         `json:"compactionAfterTokens"`
	CompactionReason       string                      `json:"compactionReason"`
	CompactionDurationMs   int64                       `json:"compactionDurationMs"`
	CompactionFailure      string                      `json:"compactionFailure"`
	Layers                 []chatsvc.ContextLayerUsage `json:"layers"`
	Source                 string                      `json:"source"`
	State                  string                      `json:"state"`
	OutputBudgetTokens     int64                       `json:"outputBudgetTokens"`
	OutputBudgetUsed       int64                       `json:"outputBudgetUsed"`
}

type wsContextCompacted struct {
	wsSessionEvent
	Usage chatsvc.ContextUsage `json:"usage"`
}

type wsSteerAccepted struct {
	wsSessionEvent
	Change chatsvc.SteeringMessage `json:"change"`
}

type wsSteerRejected struct {
	wsSessionEvent
	SubmissionID string `json:"submissionId"`
	Content      string `json:"content"`
	Error        string `json:"error"`
}

type wsPrivacy struct {
	wsSessionEvent
	llmsvc.PrivacySummary
}

func buildChatStartPayload(sessionID, turnID string, startedAt time.Time) wsStart {
	return wsStart{wsSessionEvent{"start", sessionID}, turnID, startedAt.Format(time.RFC3339Nano)}
}

func buildMessageEditedPayload(sessionID, messageID, content string, createdAt time.Time) wsMessageEdited {
	return wsMessageEdited{wsSessionEvent{"message_edited", sessionID}, messageID, content, createdAt.Format(time.RFC3339Nano)}
}

func buildChatDonePayload(sessionID string, result *chatsvc.ChatStreamResult, receivedAt, sentAt time.Time) wsDone {
	event := wsDone{
		wsSessionEvent: wsSessionEvent{"done", sessionID},
		FinishedAt:     sentAt.Format(time.RFC3339Nano),
		DurationMs:     sentAt.Sub(receivedAt).Milliseconds(),
	}
	if result == nil {
		return event
	}
	event.Answer = new(result.Answer)
	event.IsInterrupted = new(result.IsInterrupted)
	event.IsStopPlaceholder = new(result.IsStopPlaceholder)
	event.SkillLearning = result.SkillLearning
	if result.PlanID != "" {
		event.PlanID = result.PlanID
		event.PlanBody = new(result.PlanBody)
		event.Narration = new(result.Narration)
	}
	return event
}

func buildTodoUpdatePayload(sessionID string, update chatsvc.TodoUpdate, updatedAt time.Time) wsTodoUpdate {
	event := wsTodoUpdate{
		wsSessionEvent: wsSessionEvent{"todo_update", sessionID},
		Items:          update.Items, UpdatedAt: updatedAt.Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(update.Note) != "" {
		event.Note = update.Note
	}
	return event
}

func buildContextUsagePayload(sessionID string, usage chatsvc.ContextUsage) wsContextUsage {
	return wsContextUsage{
		wsSessionEvent: wsSessionEvent{"context_usage", sessionID},
		ModelConfigID:  usage.ModelConfigID, UsedTokens: usage.UsedTokens, TotalTokens: usage.TotalTokens,
		UsedPercent: usage.UsedPercent, AvailablePercent: usage.AvailablePercent,
		IsCompacted: usage.IsCompacted, CompactedAt: usage.CompactedAt,
		InputBudget: usage.InputBudget, OutputReserve: usage.OutputReserve,
		CompactionBeforeTokens: usage.CompactionBeforeTokens, CompactionAfterTokens: usage.CompactionAfterTokens,
		CompactionReason: usage.CompactionReason, CompactionDurationMs: usage.CompactionDurationMs,
		CompactionFailure: usage.CompactionFailure, Layers: usage.Layers, Source: usage.Source, State: usage.State,
		OutputBudgetTokens: usage.OutputBudgetTokens, OutputBudgetUsed: usage.OutputBudgetUsed,
	}
}

func buildContextCompactedPayload(sessionID string, usage chatsvc.ContextUsage) wsContextCompacted {
	return wsContextCompacted{wsSessionEvent{"context_compacted", sessionID}, usage}
}

func buildPostDoneContextUsagePayloads(sessionID string, usage chatsvc.ContextUsage, compactedNow bool) []any {
	payloads := []any{buildContextUsagePayload(sessionID, usage)}
	if compactedNow {
		payloads = append(payloads, buildContextCompactedPayload(sessionID, usage))
	}
	return payloads
}

func truncateWSString(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return trimmed
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func subagentIdentity(meta chatsvc.AgentEventMeta) wsSubagentIdentity {
	return wsSubagentIdentity{meta, meta.ParentToolCallID, meta.SubagentRunID}
}

func buildSubagentStartPayload(sessionID string, meta chatsvc.AgentEventMeta, title, task string) wsSubagentStart {
	return wsSubagentStart{wsSessionEvent{"subagent_start", sessionID}, subagentIdentity(meta), truncateWSString(title, 80), truncateWSString(task, 512)}
}

func toolApproval(req chatsvc.ApprovalRequest) wsToolApproval {
	return wsToolApproval{
		wsToolReview: wsToolReview{
			wsToolIdentity: wsToolIdentity{
				ToolCallID: req.ToolCallID, ToolName: req.ToolName, Command: req.Command,
				AgentEventMeta: chatsvc.AgentEventMeta{
					ParentToolCallID: req.ParentToolCallID, SubagentRunID: req.SubagentRunID,
					TeamRunID: req.TeamRunID, MemberRunID: req.MemberRunID,
				},
			},
			ReviewStatus: req.ReviewStatus, ReviewRisk: req.ReviewRisk, ReviewReason: req.ReviewReason,
		},
		Params: req.Params, RequiresApproval: req.RequiresApproval,
	}
}

func buildTeamStartPayload(run domain.TeamRun) wsTeamStart {
	return wsTeamStart{
		wsSessionEvent: wsSessionEvent{"team_start", run.SessionID},
		RequestID:      run.RequestID, TeamRunID: run.ID, Status: run.Status,
		MaxMembers: run.MaxMembers, MaxParallel: run.MaxParallel, StartedAt: run.StartedAt.Format(time.RFC3339Nano),
	}
}

func buildTeamDonePayload(run domain.TeamRun) wsTeamDone {
	event := wsTeamDone{wsTeamStart: buildTeamStartPayload(run), LastError: run.LastError}
	event.Type = "team_done"
	if run.FinishedAt != nil {
		event.FinishedAt = run.FinishedAt.Format(time.RFC3339Nano)
	}
	return event
}

func buildTeamMemberQueuedPayload(sessionID string, member domain.TeamMemberRun) wsTeamMemberQueued {
	return wsTeamMemberQueued{
		wsSessionEvent: wsSessionEvent{"team_member_queued", sessionID},
		TeamRunID:      member.TeamRunID, MemberRunID: member.ID, ToolCallID: member.ToolCallID,
		Title: truncateWSString(member.Title, 80), Task: truncateWSString(member.Task, 512),
		ModelConfigID: member.ModelConfigID, Status: member.Status,
		CreatedAt: member.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: member.UpdatedAt.Format(time.RFC3339Nano),
	}
}
