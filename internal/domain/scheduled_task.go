package domain

import "time"

const (
	ScheduledTaskStatusScheduled = "scheduled"
	ScheduledTaskStatusRunning   = "running"
	ScheduledTaskStatusPaused    = "paused"
	ScheduledTaskStatusCompleted = "completed"
	ScheduledTaskStatusError     = "error"

	ScheduledTaskRunStatusRunning     = "running"
	ScheduledTaskRunStatusInterrupted = "interrupted"
	ScheduledTaskRunStatusOK          = "ok"
	ScheduledTaskRunStatusError       = "error"
)

// ScheduledTask stores one chat-driven automation.
type ScheduledTask struct {
	ID              string     `gorm:"primaryKey;size:36" json:"id"`
	Name            string     `gorm:"size:128;not null" json:"name"`
	Prompt          string     `gorm:"type:text;not null" json:"prompt"`
	SessionID       string     `gorm:"size:36;index;not null" json:"sessionId"`
	ScheduleKind    string     `gorm:"size:16;index;not null" json:"scheduleKind"`
	RunAt           *time.Time `gorm:"index" json:"runAt,omitempty"`
	IntervalMinutes int        `gorm:"not null;default:0" json:"intervalMinutes,omitempty"`
	CronExpr        string     `gorm:"size:128" json:"cronExpr,omitempty"`
	Timezone        string     `gorm:"size:64;not null;default:'Local'" json:"timezone"`
	ModelConfigID   string     `gorm:"size:36" json:"modelConfigId,omitempty"`
	ThinkingLevel   string     `gorm:"size:16;not null;default:'off'" json:"thinkingLevel"`
	ApprovalMode    string     `gorm:"size:32;not null;default:'auto'" json:"approvalMode"`
	MaxRuns         int        `gorm:"not null;default:0" json:"maxRuns,omitempty"`
	CompletedRuns   int        `gorm:"not null;default:0" json:"completedRuns"`
	Status          string     `gorm:"size:32;index;not null" json:"status"`
	NextRunAt       *time.Time `gorm:"index" json:"nextRunAt,omitempty"`
	LastRunAt       *time.Time `gorm:"index" json:"lastRunAt,omitempty"`
	LastStatus      string     `gorm:"size:32" json:"lastStatus,omitempty"`
	LastError       string     `gorm:"type:text" json:"lastError,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// ScheduledTaskRun records one execution attempt.
type ScheduledTaskRun struct {
	ID         string     `gorm:"primaryKey;size:36" json:"id"`
	TaskName   string     `gorm:"->;-:migration" json:"taskName"`
	TaskID     string     `gorm:"size:36;index;not null" json:"taskId"`
	SessionID  string     `gorm:"size:36;index;not null" json:"sessionId"`
	RequestID  string     `gorm:"size:36;index;not null" json:"requestId"`
	Status     string     `gorm:"size:32;index;not null" json:"status"`
	Answer     string     `gorm:"type:text" json:"answer,omitempty"`
	Error      string     `gorm:"type:text" json:"error,omitempty"`
	StartedAt  time.Time  `gorm:"index;not null" json:"startedAt"`
	FinishedAt *time.Time `gorm:"index" json:"finishedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
