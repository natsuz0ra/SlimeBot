package domain

import (
	"context"
	"time"
)

const (
	BackgroundStatusRunning     = "running"
	BackgroundStatusQueued      = "queued"
	BackgroundStatusWaking      = "waking"
	BackgroundStatusCompleted   = "completed"
	BackgroundStatusFailed      = "failed"
	BackgroundStatusStopped     = "stopped"
	BackgroundStatusInterrupted = "interrupted"
)

type BackgroundTask struct {
	ID               string     `gorm:"primaryKey;size:36" json:"id"`
	TaskID           string     `gorm:"-" json:"taskId"`
	ProcessID        string     `gorm:"-" json:"processId"`
	SessionID        string     `gorm:"size:36;index;not null" json:"sessionId"`
	OriginTurnID     string     `gorm:"size:36" json:"-"`
	Command          string     `gorm:"type:text;not null" json:"command"`
	Shell            string     `json:"shell"`
	WorkingDirectory string     `gorm:"size:2048" json:"workingDirectory"`
	SandboxMode      string     `json:"sandboxMode"`
	TimeoutMs        int        `json:"timeoutMs"`
	ModelID          string     `gorm:"size:36" json:"-"`
	Thinking         string     `json:"-"`
	Subagent         string     `json:"-"`
	Surface          string     `json:"-"`
	Approval         string     `json:"-"`
	PlanMode         bool       `json:"planMode"`
	AutoWake         bool       `gorm:"not null" json:"autoWake"`
	WakeDepth        int        `json:"-"`
	Status           string     `gorm:"size:32;index;not null" json:"status"`
	Output           string     `gorm:"type:text" json:"output,omitempty"`
	Error            string     `gorm:"type:text" json:"error,omitempty"`
	ExitCode         int        `json:"exitCode"`
	StartedAt        time.Time  `json:"startedAt"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type BackgroundTaskStore interface {
	SaveBackgroundTask(context.Context, *BackgroundTask) error
	GetBackgroundTask(context.Context, string) (*BackgroundTask, error)
	ListBackgroundTasks(context.Context, string, int) ([]BackgroundTask, error)
	InterruptBackgroundTasks(context.Context, time.Time) error
}
