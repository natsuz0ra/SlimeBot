package chat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"slimebot/internal/constants"
	contextsvc "slimebot/internal/services/context"
	llmsvc "slimebot/internal/services/llm"
)

func TestToolDispatcherReplaysResultsInCallOrderAndDefersImages(t *testing.T) {
	ctx := context.Background()
	model := llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}
	agent := NewAgentService(nil, nil, nil)
	const imageURL = "data:image/png;base64,YQ=="
	run := contextsvc.Ephemeral(nil, []llmsvc.ChatMessage{{
		Role: "user", ContentParts: []llmsvc.ChatMessageContentPart{{Type: llmsvc.ChatMessageContentPartTypeImage, ImageURL: imageURL}},
	}})
	defer run.Close()
	calls := []llmsvc.ToolCallInfo{
		{ID: "todo", Name: todoUpdateFuncName, Arguments: `{"items":[{"id":"1","content":"Check order","status":"completed"}]}`},
		{ID: "list", Name: "todo__list", Arguments: "{}"},
		{ID: "plan", Name: constants.PlanStartTool},
		{ID: "image", Name: "context_read", Arguments: `{"source":1,"include_image":true}`},
		{ID: "blocked", Name: "file_read__read", Arguments: "{}"},
		{ID: "parse", Name: "todo__list", Arguments: "{"},
		{ID: "after", Name: "todo__list", Arguments: "{}"},
		{ID: "complete", Name: constants.PlanCompleteTool},
	}
	assistant := llmsvc.ChatMessage{Role: "assistant", ToolCalls: calls}
	if err := run.Append(ctx, assistant); err != nil {
		t.Fatal(err)
	}
	var todoUpdated, planStarted, planComplete bool
	dispatcher := ToolDispatcher{
		agent: agent, model: model, contextRun: run, contextReadAllowed: true,
		runtime: agent.newTurnToolRuntime("session", nil, nil),
		callbacks: AgentCallbacks{
			OnTodoUpdate: func(update TodoUpdate) error {
				todoUpdated = len(update.Items) == 1 && update.Items[0].ID == "1"
				return nil
			},
			OnPlanStart: func() error {
				if !todoUpdated || !planStarted {
					return fmt.Errorf("plan started before the ordinary tool batch finished")
				}
				return nil
			},
		},
	}
	opts := AgentLoopOptions{
		PlanStarted: &planStarted, PlanComplete: &planComplete,
		AllowedToolFunctions: map[string]struct{}{todoUpdateFuncName: {}, "todo__list": {}},
	}
	if _, err := dispatcher.dispatch(ctx, opts, []llmsvc.ChatMessage{assistant}, calls); err != nil {
		t.Fatal(err)
	}
	if !planComplete {
		t.Fatal("plan_complete did not signal completion")
	}
	prepared, err := run.Prepare(ctx, model, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	var imageAfterAllResults bool
	for _, message := range prepared.Messages {
		if message.Role == "tool" {
			ids = append(ids, message.ToolCallID)
			switch message.ToolCallID {
			case "blocked":
				if !strings.Contains(message.Content, "restricted agent run") {
					t.Errorf("admission result = %q", message.Content)
				}
			case "parse":
				if !strings.Contains(message.Content, "failed to parse arguments") {
					t.Errorf("argument result = %q", message.Content)
				}
			}
		}
		if len(message.ContentParts) == 2 && message.ContentParts[1].ImageURL == imageURL {
			imageAfterAllResults = len(ids) == len(calls)
		}
	}
	wantIDs := []string{"todo", "list", "plan", "image", "blocked", "parse", "after", "complete"}
	if !slices.Equal(ids, wantIDs) || !imageAfterAllResults {
		t.Fatalf("tool results = %v, deferred image after all results = %v", ids, imageAfterAllResults)
	}
}

func TestToolDispatcherSteeringSkipsSubsequentSpecialAndOrdinaryCalls(t *testing.T) {
	ctx := context.Background()
	run := contextsvc.Ephemeral(nil, []llmsvc.ChatMessage{{Role: "user", Content: "plan"}})
	defer run.Close()
	inbox := &turnInbox{}
	var planComplete bool
	dispatcher := ToolDispatcher{
		contextRun: run,
		callbacks: AgentCallbacks{OnPlanStart: func() error {
			inbox.mu.Lock()
			inbox.pending = append(inbox.pending, SteeringMessage{Content: "change the plan"})
			inbox.mu.Unlock()
			return nil
		}},
	}
	calls := []llmsvc.ToolCallInfo{
		{ID: "start", Name: constants.PlanStartTool},
		{ID: "complete", Name: constants.PlanCompleteTool},
		{ID: "ordinary", Name: "todo__list", Arguments: "{}"},
	}
	assistant := llmsvc.ChatMessage{Role: "assistant", ToolCalls: calls}
	if err := run.Append(ctx, assistant); err != nil {
		t.Fatal(err)
	}
	opts := AgentLoopOptions{steering: inbox, PlanComplete: &planComplete}
	if _, err := dispatcher.dispatch(ctx, opts, []llmsvc.ChatMessage{assistant}, calls); err != nil {
		t.Fatal(err)
	}
	if planComplete {
		t.Fatal("steering did not skip plan_complete")
	}
	prepared, err := run.Prepare(ctx, llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var skipped []string
	for _, message := range prepared.Messages {
		if message.Role == "tool" && strings.HasPrefix(message.Content, "Skipped because new user guidance arrived.") {
			skipped = append(skipped, message.ToolCallID)
		}
	}
	if !slices.Equal(skipped, []string{"complete", "ordinary"}) {
		t.Fatalf("skipped tool results = %v", skipped)
	}
}
