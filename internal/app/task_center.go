package app

import (
	"context"
	"slimebot/internal/domain"
)

func (c *Core) TaskCenter(ctx context.Context) (domain.TaskCenter, error) {
	result := domain.TaskCenter{ScheduledTasks: []domain.ScheduledTask{}, ScheduledRuns: []domain.ScheduledTaskRun{}, Agents: []domain.TaskCenterAgent{}, BackgroundTasks: []domain.BackgroundTask{}}
	var err error
	result.ScheduledTasks, err = c.ScheduleService.List(ctx, true)
	if err != nil {
		return result, err
	}
	page, err := c.ScheduleService.ListRuns(ctx, "", "", 50, 0)
	if err != nil {
		return result, err
	}
	result.ScheduledRuns = page.Runs
	result.Agents, err = c.Repo.ListTaskCenterAgents(ctx, 50)
	if err != nil {
		return result, err
	}
	result.BackgroundTasks, err = c.ChatService.ListBackgroundTasks(ctx, "", 100)
	return result, err
}
