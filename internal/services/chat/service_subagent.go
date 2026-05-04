package chat

import (
	"context"
	"strings"

	llmsvc "slimebot/internal/services/llm"
)

// SubagentHost is implemented by ChatService for nested agent runs.
type SubagentHost interface {
	BuildSubagentMessages(ctx context.Context, sessionID, task, parentContext string) ([]llmsvc.ChatMessage, error)
	ResolveModelRuntimeConfig(ctx context.Context, modelID string) (llmsvc.ModelRuntimeConfig, error)
}

// ResolveModelRuntimeConfig maps a stored LLM config id to runtime provider settings.
func (s *ChatService) ResolveModelRuntimeConfig(ctx context.Context, modelID string) (llmsvc.ModelRuntimeConfig, error) {
	cfg, err := s.ResolveLLMConfig(ctx, modelID)
	if err != nil {
		return llmsvc.ModelRuntimeConfig{}, err
	}
	return llmsvc.ModelRuntimeConfig{
		Provider:    cfg.Provider,
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		ContextSize: cfg.ContextSize,
	}, nil
}

// BuildSubagentMessages builds an isolated message list: system prompts + the delegated task.
func (s *ChatService) BuildSubagentMessages(ctx context.Context, sessionID, task, parentContext string) ([]llmsvc.ChatMessage, error) {
	systemPrompt, err := s.loadStableSystemPrompt()
	if err != nil {
		return nil, err
	}

	msgs := []llmsvc.ChatMessage{{Role: "system", Content: systemPrompt}}
	msgs = append(msgs, llmsvc.ChatMessage{Role: "system", Content: subagentConstraintSystemBlock()})
	if runtimeEnvPrompt := s.buildRuntimeEnvironmentPrompt(); runtimeEnvPrompt != "" {
		msgs = append(msgs, llmsvc.ChatMessage{Role: "system", Content: runtimeEnvPrompt})
	}

	var userBody strings.Builder
	userBody.WriteString("You are a sub-agent working on one delegated task for the main assistant. Complete it using available tools; you cannot delegate further.\n\n")
	userBody.WriteString("## Task\n")
	userBody.WriteString(strings.TrimSpace(task))
	if pc := strings.TrimSpace(parentContext); pc != "" {
		userBody.WriteString("\n\n## Context from main assistant\n")
		userBody.WriteString(pc)
	}
	msgs = append(msgs, llmsvc.ChatMessage{Role: "user", Content: userBody.String()})
	return msgs, nil
}

func subagentConstraintSystemBlock() string {
	return strings.TrimSpace(`## Sub-agent mode
You are running as a nested sub-agent inside a larger conversation.

Rules:
1. The tool run_subagent is not available to you; do not attempt to delegate again.
2. Execute the task directly with the tools you have.
3. Use the same language as the delegated task or context for your final report and any visible thinking/reasoning; if no language is specified, follow the task text's primary language.
4. Stay within the task and provided context; if scope is unclear, state assumptions briefly then proceed.
5. Use tools with discipline: inspect the task first, batch related inspection where possible, avoid repeated queries, and stop using tools once you have enough evidence for the requested deliverable.
6. If information remains insufficient, return confirmed facts, remaining gaps, and recommended next step instead of continuing open-ended searches.
7. Return a concise factual report as your final assistant message (the parent will use it as tool output).`)
}
