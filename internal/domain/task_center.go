package domain

import "context"

type TaskCenterAgent struct {
	AgentDescriptor
	LatestTurn *AgentTurn `json:"latestTurn,omitempty"`
}

type TaskCenter struct {
	ScheduledTasks  []ScheduledTask    `json:"scheduledTasks"`
	ScheduledRuns   []ScheduledTaskRun `json:"scheduledRuns"`
	Agents          []TaskCenterAgent  `json:"agents"`
	BackgroundTasks []BackgroundTask   `json:"backgroundTasks"`
}

type TaskCenterStore interface {
	ListTaskCenterAgents(context.Context, int) ([]TaskCenterAgent, error)
}
