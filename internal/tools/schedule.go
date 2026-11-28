package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	schedulesvc "slimebot/internal/services/schedule"
)

type scheduleTool struct{}

func init() {
	Register(&scheduleTool{})
}

func (s *scheduleTool) Name() string {
	return constants.ScheduleToolName
}

func (s *scheduleTool) Description() string {
	return "Create and manage scheduled chat tasks. Use it only after the user explicitly asks to schedule, remind, repeat, pause, resume, delete, or immediately run a task."
}

func (s *scheduleTool) Commands() []Command {
	idParam := CommandParam{Name: "task_id", Required: true, Description: "Scheduled task ID.", Example: "b9e4a2d1"}
	scheduleKind := CommandParam{
		Name:        "schedule_kind",
		Required:    true,
		Description: "Schedule kind. Use once for an exact time, interval for every N minutes, or cron for a 5-field cron expression.",
		Example:     "interval",
		Schema:      map[string]any{"type": "string", "enum": []string{"once", "interval", "cron"}},
	}
	return []Command{
		{
			Name:        "create",
			Description: "Create a scheduled chat task in the current or specified session. Convert natural language times into explicit structured parameters before calling.",
			Params: []CommandParam{
				{Name: "name", Required: true, Description: "Short user-facing task name.", Example: "每日项目摘要"},
				{Name: "prompt", Required: true, Description: "Self-contained instruction to send when the task runs.", Example: "总结项目状态并指出阻塞。"},
				{Name: "session_id", Required: false, Description: "Optional source/owner chat session ID. Each run creates a new chat session. Defaults to the current chat session.", Example: "session-id"},
				scheduleKind,
				{Name: "run_at", Required: false, Description: "RFC3339 timestamp for once schedules.", Example: "2026-05-24T09:00:00+08:00"},
				{Name: "interval_minutes", Required: false, Description: "Positive interval in minutes for interval schedules.", Example: "60", Schema: map[string]any{"type": "integer", "minimum": 1}},
				{Name: "cron_expr", Required: false, Description: "5-field cron expression for cron schedules.", Example: "0 9 * * *"},
				{Name: "model_config_id", Required: false, Description: "Optional model config ID. Defaults to global default model at run time.", Example: "model-id"},
				{Name: "thinking_level", Required: false, Description: "Optional thinking level: off, low, medium, high, max. Defaults to off.", Example: "off"},
				{Name: "approval_mode", Required: false, Description: "Optional approval mode for runs. Defaults to auto.", Example: "auto", Schema: map[string]any{"type": "string", "enum": []string{"standard", "auto_review", "auto"}}},
				{Name: "max_runs", Required: false, Description: "Optional maximum run count. 0 means unlimited.", Example: "0", Schema: map[string]any{"type": "integer", "minimum": 0}},
			},
		},
		{Name: "list", Description: "List scheduled tasks.", Params: []CommandParam{{Name: "include_inactive", Required: false, Description: "Whether to include paused and completed tasks.", Example: "true", Schema: map[string]any{"type": "boolean"}}}},
		{Name: "pause", Description: "Pause a scheduled task.", Params: []CommandParam{idParam}},
		{Name: "resume", Description: "Resume a paused scheduled task and compute a new future run time.", Params: []CommandParam{idParam}},
		{Name: "delete", Description: "Delete a scheduled task and its run history.", Params: []CommandParam{idParam}},
		{Name: "trigger", Description: "Run a scheduled task on the next scheduler tick.", Params: []CommandParam{idParam}},
		{
			Name:        "update",
			Description: "Update selected fields of a scheduled task. Omitted fields are left unchanged.",
			Params: []CommandParam{
				idParam,
				{Name: "name", Required: false, Description: "New task name.", Example: "工作日摘要"},
				{Name: "prompt", Required: false, Description: "New task prompt.", Example: "总结今天的新消息。"},
				{Name: "session_id", Required: false, Description: "New source/owner session ID. Future runs still create new chat sessions.", Example: "session-id"},
				{Name: "schedule_kind", Required: false, Description: "New schedule kind when changing schedule.", Example: "cron", Schema: map[string]any{"type": "string", "enum": []string{"once", "interval", "cron"}}},
				{Name: "run_at", Required: false, Description: "RFC3339 timestamp for once schedules.", Example: "2026-05-24T09:00:00+08:00"},
				{Name: "interval_minutes", Required: false, Description: "Positive interval in minutes for interval schedules.", Example: "60", Schema: map[string]any{"type": "integer", "minimum": 1}},
				{Name: "cron_expr", Required: false, Description: "5-field cron expression for cron schedules.", Example: "0 9 * * 1-5"},
				{Name: "model_config_id", Required: false, Description: "New model config ID, or empty to clear.", Example: "model-id"},
				{Name: "thinking_level", Required: false, Description: "New thinking level.", Example: "off"},
				{Name: "approval_mode", Required: false, Description: "New approval mode.", Example: "auto", Schema: map[string]any{"type": "string", "enum": []string{"standard", "auto_review", "auto"}}},
				{Name: "max_runs", Required: false, Description: "New maximum run count. 0 means unlimited.", Example: "0", Schema: map[string]any{"type": "integer", "minimum": 0}},
			},
		},
	}
}

func (s *scheduleTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	service, ok := scheduleServiceFromContext(ctx)
	if !ok {
		return &ExecuteResult{Error: "schedule service is not initialized"}, nil
	}
	switch strings.TrimSpace(command) {
	case "create":
		return scheduleOutput(service.Create(ctx, schedulesvc.CreateInput{
			Name:          paramStringTrim(params, "name"),
			Prompt:        paramStringTrim(params, "prompt"),
			SessionID:     sessionIDParam(ctx, params),
			Schedule:      scheduleSpecFromParams(params),
			ModelConfigID: paramStringTrim(params, "model_config_id"),
			ThinkingLevel: paramStringTrim(params, "thinking_level"),
			ApprovalMode:  paramStringTrim(params, "approval_mode"),
			MaxRuns:       intParam(params, "max_runs"),
		}))
	case "list":
		items, err := service.List(ctx, boolParam(params, "include_inactive"))
		if err != nil {
			return &ExecuteResult{Error: err.Error()}, nil
		}
		return jsonOutput(items)
	case "pause":
		if err := service.Pause(ctx, paramStringTrim(params, "task_id")); err != nil {
			return &ExecuteResult{Error: err.Error()}, nil
		}
		return jsonOutput(map[string]any{"success": true})
	case "resume":
		return scheduleOutput(service.Resume(ctx, paramStringTrim(params, "task_id")))
	case "delete":
		if err := service.Delete(ctx, paramStringTrim(params, "task_id")); err != nil {
			return &ExecuteResult{Error: err.Error()}, nil
		}
		return jsonOutput(map[string]any{"success": true})
	case "trigger":
		return scheduleOutput(service.Trigger(ctx, paramStringTrim(params, "task_id")))
	case "update":
		taskID := paramStringTrim(params, "task_id")
		input := schedulesvc.UpdateInput{}
		if hasParam(params, "name") {
			value := paramStringTrim(params, "name")
			input.Name = &value
		}
		if hasParam(params, "prompt") {
			value := paramStringTrim(params, "prompt")
			input.Prompt = &value
		}
		if hasParam(params, "session_id") {
			value := paramStringTrim(params, "session_id")
			input.SessionID = &value
		}
		spec, specErr := scheduleSpecForUpdate(ctx, service, taskID, params)
		if specErr != nil {
			return &ExecuteResult{Error: specErr.Error()}, nil
		}
		if spec != nil {
			input.Schedule = spec
		}
		if hasParam(params, "model_config_id") {
			value := paramStringTrim(params, "model_config_id")
			input.ModelConfigID = &value
		}
		if hasParam(params, "thinking_level") {
			value := paramStringTrim(params, "thinking_level")
			input.ThinkingLevel = &value
		}
		if hasParam(params, "approval_mode") {
			value := paramStringTrim(params, "approval_mode")
			input.ApprovalMode = &value
		}
		if hasParam(params, "max_runs") {
			value := intParam(params, "max_runs")
			input.MaxRuns = &value
		}
		return scheduleOutput(service.Update(ctx, taskID, input))
	default:
		return &ExecuteResult{Error: fmt.Sprintf("unknown schedule command: %s", command)}, nil
	}
}

func scheduleOutput(task any, err error) (*ExecuteResult, error) {
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return jsonOutput(task)
}

func jsonOutput(value any) (*ExecuteResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return &ExecuteResult{Output: string(data)}, nil
}

func scheduleSpecFromParams(params map[string]any) schedulesvc.ScheduleSpec {
	spec := schedulesvc.ScheduleSpec{
		Kind:            schedulesvc.ScheduleKind(paramStringTrim(params, "schedule_kind")),
		IntervalMinutes: intParam(params, "interval_minutes"),
		CronExpr:        paramStringTrim(params, "cron_expr"),
	}
	if raw := paramStringTrim(params, "run_at"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			spec.RunAt = parsed
		}
	}
	return spec
}

func scheduleSpecForUpdate(ctx context.Context, service *schedulesvc.Service, taskID string, params map[string]any) (*schedulesvc.ScheduleSpec, error) {
	if !hasAnyParam(params, "schedule_kind", "run_at", "interval_minutes", "cron_expr") {
		return nil, nil
	}
	task, err := service.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	spec := scheduleSpecFromTask(*task)
	if hasParam(params, "schedule_kind") {
		spec.Kind = schedulesvc.ScheduleKind(paramStringTrim(params, "schedule_kind"))
	}
	if hasParam(params, "run_at") {
		parsed, err := time.Parse(time.RFC3339, paramStringTrim(params, "run_at"))
		if err != nil {
			return nil, fmt.Errorf("run_at must be an RFC3339 timestamp")
		}
		spec.RunAt = parsed
	}
	if hasParam(params, "interval_minutes") {
		spec.IntervalMinutes = intParam(params, "interval_minutes")
	}
	if hasParam(params, "cron_expr") {
		spec.CronExpr = paramStringTrim(params, "cron_expr")
	}
	if hasParam(params, "schedule_kind") && string(spec.Kind) != task.ScheduleKind {
		if err := requireScheduleFieldsForKind(spec.Kind, params); err != nil {
			return nil, err
		}
	}
	return &spec, nil
}

func scheduleSpecFromTask(task domain.ScheduledTask) schedulesvc.ScheduleSpec {
	spec := schedulesvc.ScheduleSpec{
		Kind:            schedulesvc.ScheduleKind(task.ScheduleKind),
		IntervalMinutes: task.IntervalMinutes,
		CronExpr:        task.CronExpr,
		Timezone:        task.Timezone,
	}
	if task.RunAt != nil {
		spec.RunAt = *task.RunAt
	}
	return spec
}

func requireScheduleFieldsForKind(kind schedulesvc.ScheduleKind, params map[string]any) error {
	switch kind {
	case schedulesvc.ScheduleKindOnce:
		if !hasParam(params, "run_at") {
			return fmt.Errorf("run_at is required when changing to once schedule")
		}
	case schedulesvc.ScheduleKindInterval:
		if !hasParam(params, "interval_minutes") {
			return fmt.Errorf("interval_minutes is required when changing to interval schedule")
		}
	case schedulesvc.ScheduleKindCron:
		if !hasParam(params, "cron_expr") {
			return fmt.Errorf("cron_expr is required when changing to cron schedule")
		}
	}
	return nil
}

func sessionIDParam(ctx context.Context, params map[string]any) string {
	if sessionID := paramStringTrim(params, "session_id"); sessionID != "" {
		return sessionID
	}
	return currentSessionIDFromContext(ctx)
}

func hasParam(params map[string]any, key string) bool {
	if params == nil {
		return false
	}
	_, ok := params[key]
	return ok
}

func hasAnyParam(params map[string]any, keys ...string) bool {
	for _, key := range keys {
		if hasParam(params, key) {
			return true
		}
	}
	return false
}

func intParam(params map[string]any, key string) int {
	if params == nil || params[key] == nil {
		return 0
	}
	switch value := params[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(value))
		return parsed
	default:
		parsed, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprintf("%v", value)))
		return parsed
	}
}

func boolParam(params map[string]any, key string) bool {
	if params == nil || params[key] == nil {
		return false
	}
	switch value := params[key].(type) {
	case bool:
		return value
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
		return parsed
	default:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(fmt.Sprintf("%v", value)))
		return parsed
	}
}
