package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"
	schedulesvc "slimebot/internal/services/schedule"

	"github.com/google/uuid"
)

type scheduleChatRunner struct {
	core *Core
}

func (r scheduleChatRunner) RunScheduledTask(ctx context.Context, task domain.ScheduledTask) schedulesvc.RunResult {
	startedAt := time.Now()
	requestID := uuid.NewString()
	result := schedulesvc.RunResult{
		RequestID: requestID,
		StartedAt: startedAt,
	}
	if r.core == nil || r.core.ChatService == nil || r.core.Repo == nil {
		result.Success = false
		result.Error = "schedule runner is not initialized"
		result.FinishedAt = time.Now()
		return result
	}
	modelID, err := r.resolveModelID(ctx, task.ModelConfigID)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.FinishedAt = time.Now()
		return result
	}
	workingDirectory := task.WorkingDirectory
	if source, sourceErr := r.core.Repo.GetSessionByID(ctx, task.SessionID); sourceErr == nil && workingDirectory == "" {
		workingDirectory = source.WorkingDirectory
	}
	session, err := r.core.Repo.CreateTaskRunContext(ctx, task.Name, workingDirectory)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.FinishedAt = time.Now()
		return result
	}
	result.SessionID = session.ID
	runCtx, cancel := context.WithTimeout(ctx, constants.WSChatTimeout)
	defer cancel()
	approvalMode := strings.TrimSpace(task.ApprovalMode)
	if approvalMode == "" || approvalMode == constants.ApprovalModeAuto {
		approvalMode = constants.ApprovalModeScheduledAuto
	}
	streamResult, err := r.core.ChatService.HandleChatStream(
		runCtx,
		session.ID,
		requestID,
		buildScheduledTaskPrompt(task, startedAt),
		task.Prompt,
		modelID,
		nil,
		task.ThinkingLevel,
		false,
		"",
		approvalMode,
		chatsvc.AgentCallbacks{},
	)
	result.FinishedAt = time.Now()
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}
	result.Success = true
	if streamResult != nil {
		result.Answer = streamResult.Answer
	}
	return result
}

func (r scheduleChatRunner) resolveModelID(ctx context.Context, configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if _, err := r.core.ChatService.ResolveLLMConfig(ctx, configured); err != nil {
			return "", err
		}
		return configured, nil
	}
	defaultModel, err := r.core.Repo.GetSetting(ctx, constants.SettingDefaultModel)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(defaultModel) != "" {
		if _, err := r.core.ChatService.ResolveLLMConfig(ctx, defaultModel); err == nil {
			return defaultModel, nil
		}
	}
	models, err := r.core.Repo.ListLLMConfigs(ctx)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", fmt.Errorf("No available model is configured.")
	}
	return models[0].ID, nil
}

func buildScheduledTaskPrompt(task domain.ScheduledTask, runAt time.Time) string {
	return fmt.Sprintf(`[IMPORTANT: You are running as a scheduled SlimeBot task. Complete the task without asking the user for clarification unless absolutely required. If there is nothing meaningful to report, say so briefly.

Task name: %s
Scheduled task ID: %s
Run time: %s

Task instructions:
%s`, task.Name, task.ID, runAt.Format(time.RFC3339), task.Prompt)
}
