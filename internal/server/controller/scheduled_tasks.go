package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	schedulesvc "slimebot/internal/services/schedule"
	sessionsvc "slimebot/internal/services/session"
)

func (h *HTTPController) ListScheduledTasks(c WebContext) {
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	tasks, err := h.schedule.List(c.Request().Context(), true)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if tasks == nil {
		tasks = []domain.ScheduledTask{}
	}
	c.JSON(http.StatusOK, tasks)
}

func (h *HTTPController) ListScheduledTaskRuns(c WebContext) {
	status := strings.TrimSpace(c.Query("status"))
	switch status {
	case "", domain.ScheduledTaskRunStatusRunning, domain.ScheduledTaskRunStatusOK, domain.ScheduledTaskRunStatusError, domain.ScheduledTaskRunStatusInterrupted:
	default:
		jsonError(c, http.StatusBadRequest, "Invalid run status.")
		return
	}
	limit, offset := 20, 0
	for _, field := range []string{"limit", "offset"} {
		raw := strings.TrimSpace(c.Query(field))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || (field == "limit" && value == 0) {
			jsonError(c, http.StatusBadRequest, "Invalid pagination.")
			return
		}
		if field == "limit" {
			limit = min(value, 50)
		} else {
			offset = value
		}
	}
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	page, err := h.schedule.ListRuns(c.Request().Context(), c.Query("taskId"), status, limit, offset)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *HTTPController) GetScheduledTaskRun(c WebContext) {
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	run, err := h.schedule.GetRun(c.Request().Context(), c.Param("id"))
	if errors.Is(err, apperrors.ErrNotFound) {
		jsonError(c, http.StatusNotFound, "Run record not found.")
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *HTTPController) GetScheduledTaskRunHistory(c WebContext) {
	if h.schedule == nil || h.sessions == nil {
		jsonError(c, http.StatusServiceUnavailable, "Run history is unavailable.")
		return
	}
	run, err := h.schedule.GetRun(c.Request().Context(), c.Param("id"))
	if errors.Is(err, apperrors.ErrNotFound) {
		jsonError(c, http.StatusNotFound, "Run record not found.")
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if run.SessionID == "" {
		c.JSON(http.StatusOK, sessionMessagesResponse{Messages: []domain.Message{}})
		return
	}
	// Read a bounded page and allow loading older execution steps on demand.
	before, valid := parseSessionMessagesCursor(c.Query("before"))
	if !valid {
		jsonError(c, http.StatusBadRequest, "Invalid history cursor.")
		return
	}
	var beforeSeq *int64
	if raw := c.Query("beforeSeq"); raw != "" {
		seq, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || seq < 0 {
			jsonError(c, http.StatusBadRequest, "Invalid history sequence.")
			return
		}
		beforeSeq = &seq
	}
	if before != nil && beforeSeq == nil {
		jsonError(c, http.StatusBadRequest, "History sequence is required.")
		return
	}
	history, err := h.sessions.GetMessageHistory(c.Request().Context(), run.SessionID, 30, before, beforeSeq, nil, nil)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	for i := range history.Messages {
		history.Messages[i].Content = domain.StripContentMarkers(history.Messages[i].Content)
	}
	c.JSON(http.StatusOK, sessionMessagesResponse{Messages: history.Messages, ToolCallsByAssistantMessageID: history.ToolCallsByAssistantMessageID, ThinkingByAssistantMessageID: history.ThinkingByAssistantMessageID, ReplyTimingByAssistantMessageID: history.ReplyTimingByAssistantMessageID, TeamRuns: history.TeamRuns, TeamMemberRuns: history.TeamMemberRuns, HasMore: history.HasMore})
}

func (h *HTTPController) CreateScheduledTask(c WebContext) {
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	var input struct {
		Name             string                   `json:"name"`
		Prompt           string                   `json:"prompt"`
		WorkingDirectory string                   `json:"workingDirectory"`
		ModelConfigID    string                   `json:"modelConfigId"`
		Schedule         schedulesvc.ScheduleSpec `json:"schedule"`
	}
	if !bindJSONOrBadRequest(c, &input, "Invalid task payload.") {
		return
	}
	if len([]rune(input.Name)) > 128 || len([]rune(input.Prompt)) > 20000 {
		jsonError(c, http.StatusBadRequest, "Task name or instructions are too long.")
		return
	}
	if strings.TrimSpace(input.WorkingDirectory) != "" {
		directory, err := sessionsvc.ValidateWorkingDirectory(input.WorkingDirectory)
		if err != nil {
			jsonError(c, http.StatusBadRequest, "Invalid working directory.")
			return
		}
		input.WorkingDirectory = directory
	}
	if input.Schedule.Kind == schedulesvc.ScheduleKindInterval && input.Schedule.IntervalMinutes > 525600 {
		jsonError(c, http.StatusBadRequest, "Interval is too large.")
		return
	}
	if input.Schedule.Kind == schedulesvc.ScheduleKindOnce && !input.Schedule.RunAt.After(time.Now()) {
		jsonError(c, http.StatusBadRequest, "Execution time must be in the future.")
		return
	}
	task, err := h.schedule.Create(c.Request().Context(), schedulesvc.CreateInput{Name: input.Name, Prompt: input.Prompt, WorkingDirectory: input.WorkingDirectory, ModelConfigID: input.ModelConfigID, Schedule: input.Schedule})
	if err != nil {
		jsonError(c, http.StatusBadRequest, err.Error())
		return
	}
	c.JSON(http.StatusCreated, task)
}

func (h *HTTPController) ActOnScheduledTask(c WebContext) {
	if h.schedule == nil {
		jsonError(c, http.StatusServiceUnavailable, "Schedule service is unavailable.")
		return
	}
	var input struct {
		Action string `json:"action"`
	}
	if !bindJSONOrBadRequest(c, &input, "Invalid task action.") {
		return
	}
	if input.Action != "pause" && input.Action != "resume" && input.Action != "run" {
		jsonError(c, http.StatusBadRequest, "Invalid task action.")
		return
	}
	task, err := h.schedule.Get(c.Request().Context(), c.Param("id"))
	if errors.Is(err, apperrors.ErrNotFound) {
		jsonError(c, http.StatusNotFound, "Task not found.")
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if task.Status == domain.ScheduledTaskStatusRunning {
		jsonError(c, http.StatusConflict, "The task is already running.")
		return
	}
	switch input.Action {
	case "pause":
		err = h.schedule.Pause(c.Request().Context(), task.ID)
	case "resume":
		_, err = h.schedule.Resume(c.Request().Context(), task.ID)
	case "run":
		_, err = h.schedule.Trigger(c.Request().Context(), task.ID)
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	task, err = h.schedule.Get(c.Request().Context(), task.ID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, task)
}
