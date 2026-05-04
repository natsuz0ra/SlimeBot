package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llmsvc "slimebot/internal/services/llm"
)

func TestStreamChatWithToolsCapturesCompatibleReasoningContentAsThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_creation_input_tokens":5,"cache_read_input_tokens":7,"output_tokens":2}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","reasoning_content":"Need "}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","reasoning_content":"a tool."}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"exec__run","input":{}}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"pwd\"}"}}`,
			`{"type":"content_block_stop","index":1}`,
			`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}`,
			`{"type":"message_stop"}`,
		}
		for _, event := range events {
			var envelope struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal([]byte(event), &envelope); err != nil {
				t.Fatalf("failed to parse event type from %s: %v", event, err)
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", envelope.Type, event)
		}
	}))
	defer server.Close()

	client := NewAnthropicClient()
	result, err := client.StreamChatWithTools(
		context.Background(),
		llmsvc.ModelRuntimeConfig{
			Provider:      llmsvc.ProviderAnthropic,
			BaseURL:       server.URL,
			APIKey:        "key",
			Model:         "deepseek-compatible",
			ThinkingLevel: "low",
		},
		[]llmsvc.ChatMessage{{Role: "user", Content: "inspect"}},
		[]llmsvc.ToolDef{{
			Name:        "exec__run",
			Description: "Run command",
			Parameters:  map[string]any{"type": "object"},
		}},
		llmsvc.StreamCallbacks{},
	)
	if err != nil {
		t.Fatalf("StreamChatWithTools failed: %v", err)
	}
	if result.Type != llmsvc.StreamResultToolCalls {
		t.Fatalf("result type = %v, want tool calls", result.Type)
	}
	if len(result.AssistantMessage.ThinkingBlocks) != 1 {
		t.Fatalf("expected one thinking block, got %+v", result.AssistantMessage.ThinkingBlocks)
	}
	if got := result.AssistantMessage.ThinkingBlocks[0].Thinking; got != "Need a tool." {
		t.Fatalf("thinking = %q, want compatible reasoning content", got)
	}
	if len(result.ToolCalls) != 1 || !strings.Contains(result.ToolCalls[0].Arguments, "pwd") {
		t.Fatalf("tool call was not preserved: %+v", result.ToolCalls)
	}
	if result.TokenUsage == nil {
		t.Fatalf("expected token usage from message_start/message_delta")
	}
	if result.TokenUsage.InputTokens != 100 || result.TokenUsage.OutputTokens != 42 || result.TokenUsage.CacheCreationInputTokens != 5 || result.TokenUsage.CacheReadInputTokens != 7 {
		t.Fatalf("unexpected token usage: %+v", result.TokenUsage)
	}
	if got := result.TokenUsage.ContextWindowTokens(); got != 112 {
		t.Fatalf("anthropic context tokens should ignore output tokens, got %d from %+v", got, result.TokenUsage)
	}
}

func TestMergeAnthropicUsageKeepsInputAndUpdatesOutput(t *testing.T) {
	var usage llmsvc.TokenUsage
	mergeAnthropicUsage(&usage, 100, 2, 5, 7)
	mergeAnthropicUsage(&usage, 0, 42, 0, 0)
	if usage.InputTokens != 100 || usage.OutputTokens != 42 || usage.CacheCreationInputTokens != 5 || usage.CacheReadInputTokens != 7 {
		t.Fatalf("unexpected merged usage: %+v", usage)
	}
}
