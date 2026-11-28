package ws

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"slimebot/internal/domain"
	chatsvc "slimebot/internal/services/chat"
	llmsvc "slimebot/internal/services/llm"
)

func TestChatWSEventsMatchGolden(t *testing.T) {
	data, err := os.ReadFile("../../../shared/chat-protocol/fixtures/ws-events.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]json.RawMessage
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}

	const sid = "session-1"
	started := time.Date(2026, 4, 29, 1, 2, 3, 123456789, time.UTC)
	finished := started.Add(2750 * time.Millisecond)
	now := started
	var firstChunkAt time.Time
	events := map[string]any{}
	var captured any
	callbacks := (&Controller{}).buildCallbacks(func(event any) bool {
		captured = event
		return true
	}, newApprovalBroker(), sid, &firstChunkAt, func() time.Time { return now })
	capture := func(name string, err error) {
		t.Helper()
		if err != nil || captured == nil {
			t.Fatalf("%s: callback did not emit an event: %v", name, err)
		}
		events[name], captured = captured, nil
	}

	events["pong"] = wsPong{Type: "pong"}
	events["session"] = wsSession{wsSessionEvent{"session", sid}}
	events["start"] = buildChatStartPayload(sid, "turn-1", started)
	events["background_start"] = chatsvc.BackgroundStartEvent{
		Type: "start", SessionID: sid, TurnID: "wake-1", StartedAt: started.Format(time.RFC3339Nano), Background: true,
	}
	events["stopping"] = wsStopping{wsSessionEvent{"stopping", sid}}
	capture("chunk", callbacks.OnChunk("hello <world>"))
	events["done"] = buildChatDonePayload(sid, &chatsvc.ChatStreamResult{
		Answer: "result", PlanID: "plan-1", PlanBody: "# Plan",
		SkillLearning: &chatsvc.SkillLearningNotice{SkillID: "inspect", Status: "completed", Message: "Saved"},
	}, started, finished)
	events["done_without_plan"] = buildChatDonePayload(sid, &chatsvc.ChatStreamResult{
		IsInterrupted: true, IsStopPlaceholder: true,
	}, started, finished)
	events["background_done"] = chatsvc.BackgroundDoneEvent{
		Type: "done", SessionID: sid, Answer: "resumed", FinishedAt: finished.Format(time.RFC3339Nano), DurationMs: 2750,
	}
	events["error"] = wsError{Type: "error", Error: "Invalid message format."}
	events["error_with_empty_session"] = wsError{Type: "error", SessionID: new(""), Error: "sessionId is required"}
	events["message_edited"] = buildMessageEditedPayload(sid, "message-1", "edited", started)
	callbacks.OnTitleGenerated(sid, "Review")
	capture("session_title", nil)

	meta := chatsvc.AgentEventMeta{
		ParentToolCallID: "parent-1", SubagentRunID: "run-1", TeamRunID: "team-1", MemberRunID: "member-1",
	}
	capture("thinking_start", callbacks.OnThinkingStart(chatsvc.ThinkingEventMeta{}))
	capture("thinking_chunk", callbacks.OnThinkingChunk("reasoning", chatsvc.ThinkingEventMeta(meta)))
	now = finished
	capture("thinking_done", callbacks.OnThinkingDone(chatsvc.ThinkingEventMeta(meta)))
	now = started

	review := chatsvc.ApprovalReviewEvent{
		ToolCallID: "tool-1", ToolName: "exec", Command: "run",
		ReviewStatus: "needs_user", ReviewRisk: "medium", ReviewReason: "Confirm parameters",
		ParentToolCallID: meta.ParentToolCallID, SubagentRunID: meta.SubagentRunID,
		TeamRunID: meta.TeamRunID, MemberRunID: meta.MemberRunID,
	}
	approval := chatsvc.ApprovalRequest{
		ToolCallID: review.ToolCallID, ToolName: review.ToolName, Command: review.Command,
		ReviewStatus: review.ReviewStatus, ReviewRisk: review.ReviewRisk, ReviewReason: review.ReviewReason,
		ParentToolCallID: meta.ParentToolCallID, SubagentRunID: meta.SubagentRunID,
		TeamRunID: meta.TeamRunID, MemberRunID: meta.MemberRunID,
		RequiresApproval: true, Params: map[string]any{"path": "main.go"},
	}
	capture("tool_call_start", callbacks.OnToolCallStart(approval))
	capture("tool_call_review", callbacks.OnToolApprovalReview(review))
	approval.Params = nil
	capture("tool_call_approval_required", callbacks.OnToolApprovalRequired(approval))
	now = finished
	capture("tool_call_result", callbacks.OnToolCallResult(chatsvc.ToolCallResult{
		ToolCallID: review.ToolCallID, ToolName: review.ToolName, Command: review.Command,
		ParentToolCallID: meta.ParentToolCallID, SubagentRunID: meta.SubagentRunID,
		TeamRunID: meta.TeamRunID, MemberRunID: meta.MemberRunID,
		RequiresApproval: true, Status: "completed", Output: "ok",
	}))
	now = started

	team := domain.TeamRun{
		SessionID: sid, ID: "team-1", RequestID: "turn-1", Status: domain.TeamRunStatusRunning,
		MaxMembers: 8, MaxParallel: 4, StartedAt: started,
	}
	capture("team_start", callbacks.OnTeamStart(team))
	capture("team_member_queued", callbacks.OnTeamMemberQueued(domain.TeamMemberRun{
		ID: "member-1", TeamRunID: team.ID, ToolCallID: "parent-1", Title: "Research", Task: "Inspect sources",
		ModelConfigID: "model-1", Status: domain.TeamMemberRunStatusQueued, CreatedAt: started, UpdatedAt: started,
	}))
	team.Status, team.FinishedAt = domain.TeamRunStatusSucceeded, &finished
	capture("team_done", callbacks.OnTeamDone(team))
	capture("subagent_start", callbacks.OnSubagentStart(meta, "Research", "Inspect sources"))
	capture("subagent_chunk", callbacks.OnSubagentChunk(meta, "child output"))
	capture("subagent_done", callbacks.OnSubagentDone(meta, nil))
	capture("subagent_done_failed", callbacks.OnSubagentDone(meta, errors.New("cancelled")))
	capture("plan_start", callbacks.OnPlanStart())
	capture("plan_chunk", callbacks.OnPlanChunk("# "))
	capture("plan_body", callbacks.OnPlanBody("# Plan"))
	events["plan_status"] = wsPlanStatus{Type: "plan_status", PlanID: "plan-1", Status: "approved"}
	capture("todo_update", callbacks.OnTodoUpdate(chatsvc.TodoUpdate{
		Items: []chatsvc.TodoItem{{ID: "inspect", Content: "Inspect flow", Status: "in_progress"}}, Note: "working",
	}))
	capture("todo_update_no_items", callbacks.OnTodoUpdate(chatsvc.TodoUpdate{Note: " "}))
	usage := chatsvc.ContextUsage{
		SessionID: sid, ModelConfigID: "model-1", UsedTokens: 42, TotalTokens: 100, UsedPercent: 42, AvailablePercent: 58,
		IsCompacted: true, CompactedAt: started.Format(time.RFC3339Nano), InputBudget: 80, OutputReserve: 20,
		CompactionBeforeTokens: 90, CompactionAfterTokens: 42, CompactionReason: "budget", CompactionDurationMs: 20,
		Layers: []chatsvc.ContextLayerUsage{{Layer: "recent", Tokens: 42}}, Source: "provider-reported", State: "ready",
		OutputBudgetTokens: 20, OutputBudgetUsed: 12,
	}
	capture("context_usage", callbacks.OnContextUsage(usage))
	capture("context_usage_zero_fields", callbacks.OnContextUsage(chatsvc.ContextUsage{
		ModelConfigID: "model-1", TotalTokens: 100, AvailablePercent: 100,
	}))
	capture("context_compacted", callbacks.OnContextCompacted(usage))

	taskTime := started.Truncate(time.Second)
	events["background_tasks"] = chatsvc.BackgroundTasksEvent{
		Type: "background_tasks", SessionID: sid, Tasks: []domain.BackgroundTask{{
			ID: "task-1", TaskID: "task-1", ProcessID: "process-1", SessionID: sid, Command: "build", Shell: "sh",
			TimeoutMs: 1000, AutoWake: true, Status: "queued", StartedAt: taskTime, CreatedAt: taskTime, UpdatedAt: taskTime,
		}},
	}
	events["background_task"] = chatsvc.BackgroundTaskEvent{
		Type: "background_task", SessionID: sid, TaskID: "task-1", ProcessID: "process-1", Status: "queued",
	}
	events["background_error"] = chatsvc.BackgroundErrorEvent{
		Type: "background_error", SessionID: sid, Error: "Continuation failed.",
	}
	events["steer_accepted"] = wsSteerAccepted{wsSessionEvent{"steer_accepted", sid}, chatsvc.SteeringMessage{
		SubmissionID: "submission-1", MessageID: "message-1", TurnID: "turn-1", Content: "adjust", CreatedAt: started,
	}}
	events["steer_rejected"] = wsSteerRejected{wsSessionEvent{"steer_rejected", sid}, "submission-2", "", "stale turn"}
	capture("privacy", callbacks.OnPrivacy(llmsvc.PrivacySummary{Masked: 3, Warned: 1}))

	if len(events) != len(golden) {
		t.Fatalf("produced %d events, want %d", len(events), len(golden))
	}
	for name, expected := range golden {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(events[name])
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(expected, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %s\nwant %s", encoded, expected)
			}
		})
	}
}
