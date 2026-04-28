package chat

import (
	"context"
	"testing"

	"slimebot/internal/constants"
	llmsvc "slimebot/internal/services/llm"
)

func TestHandleRunSubagentTool_PlanModeChildKeepsReadOnlyToolFilter(t *testing.T) {
	provider := &captureToolDefsProvider{}
	agent := NewAgentService(llmsvc.NewFactory(provider), nil, nil, nil)
	agent.SetSubagentHost(stubSubagentHost{})

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

type captureToolDefsProvider struct {
	toolDefs []llmsvc.ToolDef
}

func (p *captureToolDefsProvider) StreamChatWithTools(
	_ context.Context,
	_ llmsvc.ModelRuntimeConfig,
	_ []llmsvc.ChatMessage,
	toolDefs []llmsvc.ToolDef,
	callbacks llmsvc.StreamCallbacks,
) (*llmsvc.StreamResult, error) {
	p.toolDefs = append([]llmsvc.ToolDef{}, toolDefs...)
	if callbacks.OnChunk != nil {
		if err := callbacks.OnChunk("child answer"); err != nil {
			return nil, err
		}
	}
	return &llmsvc.StreamResult{Type: llmsvc.StreamResultText}, nil
}

type stubSubagentHost struct{}

func (stubSubagentHost) BuildSubagentMessages(_ context.Context, _, task, parentContext string) ([]llmsvc.ChatMessage, error) {
	return []llmsvc.ChatMessage{{Role: "user", Content: task + parentContext}}, nil
}

func (stubSubagentHost) ResolveModelRuntimeConfig(_ context.Context, _ string) (llmsvc.ModelRuntimeConfig, error) {
	return llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI}, nil
}
