package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"slimebot/internal/constants"
	llmsvc "slimebot/internal/services/llm"
)

func TestHandleRunSubagentTool_PlanModeChildKeepsReadOnlyToolFilter(t *testing.T) {
	provider := &captureToolDefsProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	agent.SetSubagentHost(&stubSubagentHost{})

	messages := []llmsvc.ChatMessage{{Role: "user", Content: "make a plan"}}
	err := agent.handleRunSubagentTool(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		nil,
		map[string]struct{}{},
		AgentCallbacks{},
		AgentLoopOptions{PlanMode: true},
		llmsvc.ToolCallInfo{ID: "call-subagent", Name: constants.RunSubagentTool},
		resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
		map[string]string{"task": "Inspect read-only context"},
		"",
		"",
		&messages,
	)
	if err != nil {
		t.Fatalf("handleRunSubagentTool failed: %v", err)
	}

	if len(provider.toolDefs) == 0 {
		t.Fatal("expected child agent tool definitions to be captured")
	}
	if containsToolName(provider.toolDefs, "exec__run") {
		t.Fatalf("plan-mode child agent must not receive exec__run; got %#v", toolNames(provider.toolDefs))
	}
	if containsToolName(provider.toolDefs, constants.RunSubagentTool) {
		t.Fatalf("child agent must not receive nested run_subagent; got %#v", toolNames(provider.toolDefs))
	}
	if !containsToolName(provider.toolDefs, "web_search__search") {
		t.Fatalf("plan-mode child agent should keep read-only web_search; got %#v", toolNames(provider.toolDefs))
	}
}

func TestWrapSubagentCallbacksTagsThinkingEvents(t *testing.T) {
	var starts []ThinkingEventMeta
	var chunks []ThinkingEventMeta
	var done []ThinkingEventMeta
	base := AgentCallbacks{
		OnThinkingStart: func(meta ThinkingEventMeta) error {
			starts = append(starts, meta)
			return nil
		},
		OnThinkingChunk: func(_ string, meta ThinkingEventMeta) error {
			chunks = append(chunks, meta)
			return nil
		},
		OnThinkingDone: func(meta ThinkingEventMeta) error {
			done = append(done, meta)
			return nil
		},
	}

	wrapped := wrapSubagentCallbacks(base, "parent-tool", "sub-run")
	if err := wrapped.OnThinkingStart(ThinkingEventMeta{}); err != nil {
		t.Fatalf("OnThinkingStart failed: %v", err)
	}
	if err := wrapped.OnThinkingChunk("thought", ThinkingEventMeta{}); err != nil {
		t.Fatalf("OnThinkingChunk failed: %v", err)
	}
	if err := wrapped.OnThinkingDone(ThinkingEventMeta{}); err != nil {
		t.Fatalf("OnThinkingDone failed: %v", err)
	}

	want := ThinkingEventMeta{ParentToolCallID: "parent-tool", SubagentRunID: "sub-run"}
	if len(starts) != 1 || starts[0] != want {
		t.Fatalf("unexpected thinking starts: %+v", starts)
	}
	if len(chunks) != 1 || chunks[0] != want {
		t.Fatalf("unexpected thinking chunks: %+v", chunks)
	}
	if len(done) != 1 || done[0] != want {
		t.Fatalf("unexpected thinking done: %+v", done)
	}
}

func TestHandleRunSubagentTool_EmitsNormalizedSubagentTitle(t *testing.T) {
	provider := &captureToolDefsProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	agent.SetSubagentHost(&stubSubagentHost{})

	var gotTitle string
	var gotTask string
	messages := []llmsvc.ChatMessage{{Role: "user", Content: "delegate"}}
	err := agent.handleRunSubagentTool(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		nil,
		map[string]struct{}{},
		AgentCallbacks{
			OnSubagentStart: func(_ string, _ string, title string, task string) error {
				gotTitle = title
				gotTask = task
				return nil
			},
		},
		AgentLoopOptions{},
		llmsvc.ToolCallInfo{ID: "call-subagent", Name: constants.RunSubagentTool},
		resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
		map[string]string{
			"title": "  Inspect\nprompt   flow  ",
			"task":  "Inspect prompt flow and report risks",
		},
		"",
		"",
		&messages,
	)
	if err != nil {
		t.Fatalf("handleRunSubagentTool failed: %v", err)
	}
	if gotTitle != "Inspect prompt flow" {
		t.Fatalf("unexpected title: %q", gotTitle)
	}
	if gotTask != "Inspect prompt flow and report risks" {
		t.Fatalf("unexpected task: %q", gotTask)
	}
}

func TestHandleRunSubagentTool_FallsBackTitleToTask(t *testing.T) {
	provider := &captureToolDefsProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	agent.SetSubagentHost(&stubSubagentHost{})

	var gotTitle string
	longTask := strings.Repeat("a", 90) + "\nmore detail"
	messages := []llmsvc.ChatMessage{{Role: "user", Content: "delegate"}}
	err := agent.handleRunSubagentTool(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		nil,
		map[string]struct{}{},
		AgentCallbacks{
			OnSubagentStart: func(_ string, _ string, title string, _ string) error {
				gotTitle = title
				return nil
			},
		},
		AgentLoopOptions{},
		llmsvc.ToolCallInfo{ID: "call-subagent", Name: constants.RunSubagentTool},
		resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
		map[string]string{"task": longTask},
		"",
		"",
		&messages,
	)
	if err != nil {
		t.Fatalf("handleRunSubagentTool failed: %v", err)
	}
	if len([]rune(gotTitle)) > 80 {
		t.Fatalf("fallback title should be capped to 80 runes, got %d: %q", len([]rune(gotTitle)), gotTitle)
	}
	if !strings.HasSuffix(gotTitle, "...") {
		t.Fatalf("fallback title should show truncation suffix: %q", gotTitle)
	}
}

func TestRunAgentLoop_HandlesTodoUpdateWithoutRegularToolCallback(t *testing.T) {
	provider := &todoUpdateProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	var updates []TodoUpdate
	var toolStarts int

	answer, err := agent.RunAgentLoop(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		[]llmsvc.ChatMessage{{Role: "user", Content: "do several things"}},
		nil,
		map[string]struct{}{},
		AgentCallbacks{
			OnTodoUpdate: func(update TodoUpdate) error {
				updates = append(updates, update)
				return nil
			},
			OnToolCallStart: func(req ApprovalRequest) error {
				toolStarts++
				return nil
			},
		},
		AgentLoopOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgentLoop failed: %v", err)
	}
	if answer != "done" {
		t.Fatalf("unexpected final answer: %q", answer)
	}
	if toolStarts != 0 {
		t.Fatalf("todo_update must not emit regular tool starts, got %d", toolStarts)
	}
	if len(updates) != 1 {
		t.Fatalf("expected one todo update, got %d", len(updates))
	}
	if updates[0].Note != "starting" {
		t.Fatalf("unexpected note: %q", updates[0].Note)
	}
	if len(updates[0].Items) != 2 {
		t.Fatalf("expected two todo items, got %+v", updates[0].Items)
	}
	if updates[0].Items[0].Status != "in_progress" || updates[0].Items[1].Status != "pending" {
		t.Fatalf("unexpected statuses: %+v", updates[0].Items)
	}
	if len(provider.seenMessages) < 3 || provider.seenMessages[2].Role != "tool" {
		t.Fatalf("expected todo tool response in follow-up context, got %+v", provider.seenMessages)
	}
}

type todoUpdateProvider struct {
	calls        int
	seenMessages []llmsvc.ChatMessage
}

func (p *todoUpdateProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	toolDefs []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.calls++
	p.seenMessages = append([]llmsvc.ChatMessage{}, messages...)
	if !containsToolName(toolDefs, constants.TodoUpdateTool) {
		return nil, fmt.Errorf("missing %s tool", constants.TodoUpdateTool)
	}
	if p.calls == 1 {
		return &llmsvc.StreamResult{
			Type: llmsvc.StreamResultToolCalls,
			ToolCalls: []llmsvc.ToolCallInfo{{
				ID:        "todo-call-1",
				Name:      constants.TodoUpdateTool,
				Arguments: `{"note":"starting","items":[{"id":"1","content":"Inspect","status":"in_progress"},{"id":"2","content":"Implement","status":"pending"}]}`,
			}},
			AssistantMessage: llmsvc.ChatMessage{
				Role: "assistant",
				ToolCalls: []llmsvc.ToolCallInfo{{
					ID:        "todo-call-1",
					Name:      constants.TodoUpdateTool,
					Arguments: `{"note":"starting","items":[{"id":"1","content":"Inspect","status":"in_progress"},{"id":"2","content":"Implement","status":"pending"}]}`,
				}},
			},
		}, nil
	}
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk("done"); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

type captureToolDefsProvider struct {
	toolDefs     []llmsvc.ToolDef
	modelConfigs []llmsvc.ModelRuntimeConfig
}

func (p *captureToolDefsProvider) StreamChatWithTools(
	_ context.Context,
	modelConfig llmsvc.ModelRuntimeConfig,
	_ []llmsvc.ChatMessage,
	toolDefs []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.toolDefs = append([]llmsvc.ToolDef{}, toolDefs...)
	p.modelConfigs = append(p.modelConfigs, modelConfig)
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk("child answer"); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

func TestHandleRunSubagentTool_InheritsParentModelForEmptyOrDefaultModelID(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
	}{
		{name: "empty", params: map[string]string{"task": "Inspect inherited model"}},
		{name: "default", params: map[string]string{"task": "Inspect inherited model", "model_id": "default"}},
		{name: "llm-invented-fast", params: map[string]string{"task": "Inspect inherited model", "model_id": "fast"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &captureToolDefsProvider{}
			agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
			host := &stubSubagentHost{}
			agent.SetSubagentHost(host)

			parentModel := llmsvc.ModelRuntimeConfig{
				Provider:      llmsvc.ProviderOpenAI,
				BaseURL:       "https://parent.example/v1",
				APIKey:        "parent-key",
				Model:         "parent-model",
				ThinkingLevel: "high",
			}
			messages := []llmsvc.ChatMessage{{Role: "user", Content: "delegate"}}
			err := agent.handleRunSubagentTool(
				context.Background(),
				parentModel,
				"session-1",
				nil,
				map[string]struct{}{},
				AgentCallbacks{},
				AgentLoopOptions{},
				llmsvc.ToolCallInfo{ID: "call-subagent", Name: constants.RunSubagentTool},
				resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
				tt.params,
				"",
				"",
				&messages,
			)
			if err != nil {
				t.Fatalf("handleRunSubagentTool failed: %v", err)
			}
			if host.resolveCalls != 0 {
				t.Fatalf("expected inherited model without resolving override, got %d resolve calls", host.resolveCalls)
			}
			if len(provider.modelConfigs) != 1 {
				t.Fatalf("expected one child model config, got %d", len(provider.modelConfigs))
			}
			if got := provider.modelConfigs[0]; got != parentModel {
				t.Fatalf("child model did not inherit parent config:\n got=%+v\nwant=%+v", got, parentModel)
			}
		})
	}
}

func TestHandleRunSubagentTool_UserConfiguredModelKeepsParentThinkingLevel(t *testing.T) {
	provider := &captureToolDefsProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	host := &stubSubagentHost{
		resolved: llmsvc.ModelRuntimeConfig{
			Provider: llmsvc.ProviderAnthropic,
			BaseURL:  "https://child.example/v1",
			APIKey:   "child-key",
			Model:    "child-model",
		},
	}
	agent.SetSubagentHost(host)

	messages := []llmsvc.ChatMessage{{Role: "user", Content: "delegate"}}
	err := agent.handleRunSubagentTool(
		context.Background(),
		llmsvc.ModelRuntimeConfig{
			Provider:      llmsvc.ProviderOpenAI,
			BaseURL:       "https://parent.example/v1",
			APIKey:        "parent-key",
			Model:         "parent-model",
			ThinkingLevel: "medium",
		},
		"session-1",
		nil,
		map[string]struct{}{},
		AgentCallbacks{},
		AgentLoopOptions{},
		llmsvc.ToolCallInfo{ID: "call-subagent", Name: constants.RunSubagentTool},
		resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
		map[string]string{"task": "Inspect override"},
		"child-config",
		"",
		&messages,
	)
	if err != nil {
		t.Fatalf("handleRunSubagentTool failed: %v", err)
	}
	if host.resolveCalls != 1 || host.lastModelID != "child-config" {
		t.Fatalf("expected one override resolve for child-config, got calls=%d id=%q", host.resolveCalls, host.lastModelID)
	}
	if len(provider.modelConfigs) != 1 {
		t.Fatalf("expected one child model config, got %d", len(provider.modelConfigs))
	}
	want := host.resolved
	want.ThinkingLevel = "medium"
	if got := provider.modelConfigs[0]; got != want {
		t.Fatalf("child override did not keep parent thinking level:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestHandleRunSubagentTool_PreservesChildReasoningAcrossToolIterations(t *testing.T) {
	provider := &subagentReasoningIterationProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	agent.SetSubagentHost(&stubSubagentHost{})

	messages := []llmsvc.ChatMessage{{Role: "user", Content: "delegate"}}
	err := agent.handleRunSubagentTool(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		nil,
		map[string]struct{}{},
		AgentCallbacks{},
		AgentLoopOptions{},
		llmsvc.ToolCallInfo{ID: "call-subagent", Name: constants.RunSubagentTool},
		resolvedToolInvocation{toolName: constants.RunSubagentTool, command: "run"},
		map[string]string{"task": "Inspect with reasoning"},
		"",
		"",
		&messages,
	)
	if err != nil {
		t.Fatalf("handleRunSubagentTool failed: %v", err)
	}
	if provider.secondCallAssistant == nil {
		t.Fatal("expected child second model call to include prior assistant message")
	}
	if got := provider.secondCallAssistant.ReasoningContent; got != "Child needs a tool." {
		t.Fatalf("child reasoning content was not preserved: %q", got)
	}
}

func TestRunAgentLoop_SubagentCanRunBeyondFormerThirtyIterationCap(t *testing.T) {
	provider := &loopUntilTextProvider{textAtCall: 31}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)

	answer, err := agent.RunAgentLoop(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		[]llmsvc.ChatMessage{{Role: "user", Content: "keep going"}},
		nil,
		map[string]struct{}{},
		AgentCallbacks{},
		AgentLoopOptions{Depth: 1},
	)
	if err != nil {
		t.Fatalf("subagent should not stop at the former 30-iteration cap: %v", err)
	}
	if answer != "done after loop" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if provider.call != 31 {
		t.Fatalf("expected 31 model calls, got %d", provider.call)
	}
}

func TestRunAgentLoop_MainAgentStillStopsAtMaxIterations(t *testing.T) {
	provider := &loopUntilTextProvider{textAtCall: constants.AgentMaxIterations + 1}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)

	_, err := agent.RunAgentLoop(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI},
		"session-1",
		[]llmsvc.ChatMessage{{Role: "user", Content: "keep going"}},
		nil,
		map[string]struct{}{},
		AgentCallbacks{},
		AgentLoopOptions{},
	)
	if err == nil {
		t.Fatal("expected main agent to stop at max iterations")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("max iterations (%d)", constants.AgentMaxIterations)) {
		t.Fatalf("unexpected max iteration error: %v", err)
	}
	if provider.call != constants.AgentMaxIterations {
		t.Fatalf("expected %d model calls, got %d", constants.AgentMaxIterations, provider.call)
	}
}

type stubSubagentHost struct {
	resolved     llmsvc.ModelRuntimeConfig
	resolveErr   error
	resolveCalls int
	lastModelID  string
}

type subagentReasoningIterationProvider struct {
	call                int
	secondCallAssistant *llmsvc.ChatMessage
}

type loopUntilTextProvider struct {
	call       int
	textAtCall int
}

func (p *loopUntilTextProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	_ []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.call++
	if p.call >= p.textAtCall {
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk("done after loop"); err != nil {
				return nil, err
			}
		}
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
	}
	return &llmsvc.StreamResult{
		Type: llmsvc.StreamResultToolCalls,
		ToolCalls: []llmsvc.ToolCallInfo{{
			ID:        fmt.Sprintf("plan-start-%d", p.call),
			Name:      constants.PlanStartTool,
			Arguments: "{}",
		}},
		AssistantMessage: llmsvc.ChatMessage{
			Role: "assistant",
			ToolCalls: []llmsvc.ToolCallInfo{{
				ID:        fmt.Sprintf("plan-start-%d", p.call),
				Name:      constants.PlanStartTool,
				Arguments: "{}",
			}},
		},
	}, nil
}

func (p *subagentReasoningIterationProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	messages []llmsvc.ChatMessage,
	_ []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.call++
	switch p.call {
	case 1:
		return &llmsvc.StreamResult{
			Type: llmsvc.StreamResultToolCalls,
			ToolCalls: []llmsvc.ToolCallInfo{{
				ID:        "plan-start-call",
				Name:      constants.PlanStartTool,
				Arguments: "{}",
			}},
			AssistantMessage: llmsvc.ChatMessage{
				Role:             "assistant",
				ReasoningContent: "Child needs a tool.",
				ToolCalls: []llmsvc.ToolCallInfo{{
					ID:        "plan-start-call",
					Name:      constants.PlanStartTool,
					Arguments: "{}",
				}},
			},
		}, nil
	default:
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" {
				msg := messages[i]
				p.secondCallAssistant = &msg
				break
			}
		}
		if callbacks.OnChunk != nil {
			if err := callbacks.OnChunk("child done"); err != nil {
				return nil, err
			}
		}
		return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
	}
}

func (*stubSubagentHost) BuildSubagentMessages(_ context.Context, _, task, parentContext string) ([]llmsvc.ChatMessage, error) {
	return []llmsvc.ChatMessage{{Role: "user", Content: task + parentContext}}, nil
}

func (h *stubSubagentHost) ResolveModelRuntimeConfig(_ context.Context, modelID string) (llmsvc.ModelRuntimeConfig, error) {
	h.resolveCalls++
	h.lastModelID = modelID
	if h.resolveErr != nil {
		return llmsvc.ModelRuntimeConfig{}, h.resolveErr
	}
	if h.resolved.Model == "" {
		return llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI, Model: fmt.Sprintf("resolved-%s", modelID)}, nil
	}
	return h.resolved, nil
}
