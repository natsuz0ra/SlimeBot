package anthropic

import (
	"encoding/json"
	"testing"

	llmsvc "slimebot/internal/services/llm"
)

func TestBuildAssistantBlocksPreservesThinkingBeforeToolUse(t *testing.T) {
	blocks := buildAssistantBlocks(llmsvc.ChatMessage{
		Role:    "assistant",
		Content: "I will inspect the file.",
		ThinkingBlocks: []llmsvc.ThinkingBlockInfo{{
			Thinking:  "Need to inspect before answering.",
			Signature: "sig-1",
		}},
		ToolCalls: []llmsvc.ToolCallInfo{{
			ID:        "toolu_1",
			Name:      "exec__run",
			Arguments: `{"command":"pwd"}`,
		}},
	})

	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal blocks failed: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal blocks failed: %v\njson=%s", err, raw)
	}
	if len(got) != 3 {
		t.Fatalf("expected thinking, text, tool_use blocks, got %d: %s", len(got), raw)
	}
	if got[0]["type"] != "thinking" {
		t.Fatalf("expected first block to be thinking, got %v: %s", got[0]["type"], raw)
	}
	if got[0]["thinking"] != "Need to inspect before answering." {
		t.Fatalf("thinking content was not preserved: %#v", got[0])
	}
	if got[0]["signature"] != "sig-1" {
		t.Fatalf("thinking signature was not preserved: %#v", got[0])
	}
	if got[1]["type"] != "text" {
		t.Fatalf("expected second block to be text, got %v: %s", got[1]["type"], raw)
	}
	if got[2]["type"] != "tool_use" {
		t.Fatalf("expected third block to be tool_use, got %v: %s", got[2]["type"], raw)
	}
}

func TestBuildAssistantBlocksPreservesThinkingWithoutSignature(t *testing.T) {
	blocks := buildAssistantBlocks(llmsvc.ChatMessage{
		Role: "assistant",
		ThinkingBlocks: []llmsvc.ThinkingBlockInfo{{
			Thinking: "DeepSeek-style thinking without an Anthropic signature.",
		}},
		ToolCalls: []llmsvc.ToolCallInfo{{
			ID:        "toolu_1",
			Name:      "exec__run",
			Arguments: `{"command":"pwd"}`,
		}},
	})

	raw, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal blocks failed: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal blocks failed: %v\njson=%s", err, raw)
	}
	if len(got) != 2 {
		t.Fatalf("expected thinking and tool_use blocks, got %d: %s", len(got), raw)
	}
	if got[0]["type"] != "thinking" {
		t.Fatalf("expected first block to be thinking, got %v: %s", got[0]["type"], raw)
	}
	if got[0]["thinking"] != "DeepSeek-style thinking without an Anthropic signature." {
		t.Fatalf("thinking content was not preserved: %#v", got[0])
	}
	if _, ok := got[0]["signature"]; ok {
		t.Fatalf("signature should be omitted when unavailable for compatible providers: %#v", got[0])
	}
}

func TestBuildAnthropicToolsUsesToolParametersAsInputSchema(t *testing.T) {
	tools := buildAnthropicTools([]llmsvc.ToolDef{{
		Name:        "activate_skill",
		Description: "Load a skill guide by name.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to activate.",
					"enum":        []any{"systematic-debugging"},
				},
			},
			"required": []string{"name"},
		},
	}})

	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal tools failed: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal tools failed: %v\njson=%s", err, raw)
	}
	if len(got) != 1 {
		t.Fatalf("expected one tool, got %d: %s", len(got), raw)
	}

	inputSchema, ok := got[0]["input_schema"].(map[string]any)
	if !ok {
		t.Fatalf("input_schema missing or invalid: %#v\njson=%s", got[0]["input_schema"], raw)
	}
	if inputSchema["type"] != "object" {
		t.Fatalf("input_schema.type = %v, want object: %s", inputSchema["type"], raw)
	}

	properties, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input_schema.properties missing or invalid: %#v\njson=%s", inputSchema["properties"], raw)
	}
	nameSchema, ok := properties["name"].(map[string]any)
	if !ok {
		t.Fatalf("properties.name missing or invalid: %#v\njson=%s", properties["name"], raw)
	}
	if nameSchema["type"] != "string" {
		t.Fatalf("properties.name.type = %v, want string: %s", nameSchema["type"], raw)
	}
	if _, nested := properties["required"]; nested {
		t.Fatalf("input_schema.required was incorrectly nested under properties: %s", raw)
	}

	required, ok := inputSchema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "name" {
		t.Fatalf("input_schema.required = %#v, want [name]: %s", inputSchema["required"], raw)
	}
}
