package domain

import "time"

type ScheduleKind string

const (
	ScheduleKindOnce     ScheduleKind = "once"
	ScheduleKindInterval ScheduleKind = "interval"
	ScheduleKindCron     ScheduleKind = "cron"
)

type ScheduleSpec struct {
	Kind            ScheduleKind `json:"kind"`
	RunAt           time.Time    `json:"runAt,omitempty"`
	IntervalMinutes int          `json:"intervalMinutes,omitempty"`
	CronExpr        string       `json:"cronExpr,omitempty"`
	Timezone        string       `json:"timezone,omitempty"`
}

type ScheduledTaskCreateInput struct {
	WorkingDirectory string
	Name             string
	Prompt           string
	SessionID        string
	Schedule         ScheduleSpec
	ModelConfigID    string
	ThinkingLevel    string
	ApprovalMode     string
	MaxRuns          int
}

type ScheduledTaskUpdateInput struct {
	Name          *string
	Prompt        *string
	SessionID     *string
	Schedule      *ScheduleSpec
	ModelConfigID *string
	ThinkingLevel *string
	ApprovalMode  *string
	MaxRuns       *int
}
