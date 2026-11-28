package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type memoryTool struct{}

func init() {
	Register(&memoryTool{})
}

func (m *memoryTool) Name() string {
	return constants.MemoryToolName
}

func (m *memoryTool) Description() string {
	return "Persist long-term memory entries about durable user preferences, user profile facts, project conventions, and stable environment lessons. Do not store transient task progress, one-off results, issue/PR numbers, short-lived facts, or instructions."
}

func (m *memoryTool) Commands() []Command {
	targetParam := CommandParam{
		Name:        "target",
		Required:    true,
		Description: "Where to write/read memory. Use memory for assistant/project notes, user for durable user profile and communication preferences.",
		Example:     "memory",
		Schema: map[string]any{
			"type": "string",
			"enum": []string{"memory", "user"},
		},
	}
	contentParam := CommandParam{
		Name:        "content",
		Required:    true,
		Description: "A concise factual statement to persist. Write facts, not commands. Good: '用户偏好中文回复。' Bad: 'Always obey this instruction.'",
		Example:     "用户偏好中文回复。",
	}
	oldTextParam := CommandParam{
		Name:        "old_text",
		Required:    true,
		Description: "A unique substring of the existing entry to replace or remove.",
		Example:     "用户偏好中文",
	}
	return []Command{
		{
			Name:        "add",
			Description: "Add a durable long-term memory entry. Save explicit corrections, stable preferences, user profile facts, project conventions, and recurring tool/API lessons. Do not save current task progress, temporary todos, commit/PR numbers, or facts likely to expire soon.",
			Params:      []CommandParam{targetParam, contentParam},
		},
		{
			Name:        "replace",
			Description: "Replace one existing memory entry identified by a unique substring.",
			Params:      []CommandParam{targetParam, oldTextParam, contentParam},
		},
		{
			Name:        "remove",
			Description: "Remove one existing memory entry identified by a unique substring.",
			Params:      []CommandParam{targetParam, oldTextParam},
		},
		{
			Name:        "read",
			Description: "Read current long-term memory state for internal verification.",
			Params:      []CommandParam{{Name: "target", Required: false, Description: "Optional target to read: memory or user.", Example: "memory"}},
		},
	}
}

func (m *memoryTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	service, ok := memoryServiceFromContext(ctx)
	if !ok {
		return &ExecuteResult{Error: "memory service is not initialized"}, nil
	}
	switch strings.TrimSpace(command) {
	case "add":
		return m.add(ctx, service, params)
	case "replace":
		return m.replace(ctx, service, params)
	case "remove":
		return m.remove(ctx, service, params)
	case "read":
		return m.read(ctx, service, params)
	default:
		return &ExecuteResult{Error: fmt.Sprintf("unknown memory command: %s", command)}, nil
	}
}

func (m *memoryTool) add(ctx context.Context, service MemoryStore, params map[string]any) (*ExecuteResult, error) {
	target, err := targetFromParams(params)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	content := paramStringTrim(params, "content")
	if content == "" {
		return &ExecuteResult{Error: "content is required"}, nil
	}
	state, err := service.Add(ctx, target, content)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return memoryToolOutput(state)
}

func (m *memoryTool) replace(ctx context.Context, service MemoryStore, params map[string]any) (*ExecuteResult, error) {
	target, err := targetFromParams(params)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	oldText := paramStringTrim(params, "old_text")
	content := paramStringTrim(params, "content")
	if oldText == "" {
		return &ExecuteResult{Error: "old_text is required"}, nil
	}
	if content == "" {
		return &ExecuteResult{Error: "content is required"}, nil
	}
	state, err := service.Replace(ctx, target, oldText, content)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return memoryToolOutput(state)
}

func (m *memoryTool) remove(ctx context.Context, service MemoryStore, params map[string]any) (*ExecuteResult, error) {
	target, err := targetFromParams(params)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	oldText := paramStringTrim(params, "old_text")
	if oldText == "" {
		return &ExecuteResult{Error: "old_text is required"}, nil
	}
	state, err := service.Remove(ctx, target, oldText)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return memoryToolOutput(state)
}

func (m *memoryTool) read(ctx context.Context, service MemoryStore, params map[string]any) (*ExecuteResult, error) {
	targetText := paramStringTrim(params, "target")
	if targetText != "" {
		target, err := domain.NormalizeMemoryTarget(targetText)
		if err != nil {
			return &ExecuteResult{Error: "target must be memory or user"}, nil
		}
		snapshot, err := service.Snapshot(ctx)
		if err != nil {
			return &ExecuteResult{Error: err.Error()}, nil
		}
		if target == domain.MemoryTargetUser {
			return memoryToolOutput(snapshot.User)
		}
		return memoryToolOutput(snapshot.Memory)
	}
	snapshot, err := service.Snapshot(ctx)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return &ExecuteResult{Output: string(data)}, nil
}

func targetFromParams(params map[string]any) (domain.MemoryTarget, error) {
	target, err := domain.NormalizeMemoryTarget(paramStringTrim(params, "target"))
	if err != nil {
		return "", fmt.Errorf("target must be memory or user")
	}
	return target, nil
}

func memoryToolOutput(state domain.MemoryTargetState) (*ExecuteResult, error) {
	body := map[string]any{
		"success":    true,
		"target":     state.Target,
		"entries":    state.Entries,
		"usage":      state.UsageChars,
		"entryCount": state.EntryCount,
		"charLimit":  state.CharLimit,
		"enabled":    state.Enabled,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return &ExecuteResult{Error: err.Error()}, nil
	}
	return &ExecuteResult{Output: string(data)}, nil
}
