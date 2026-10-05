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
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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
		[]llmsvc.ChatMessage{{Role: "user", ContentParts: []llmsvc.ChatMessageContentPart{{Type: llmsvc.ChatMessageContentPartTypeText, Text: "  inspect 🚀\n "}}}},
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
	if request["max_tokens"] != float64(llmsvc.ThinkingBudgetTokens("low")+1) || result.FinishReason != "tool_use" {
		t.Fatalf("thinking output budget/termination lost: request=%v finish=%s", request, result.FinishReason)
	}
	messages := request["messages"].([]any)
	parts := messages[0].(map[string]any)["content"].([]any)
	if parts[0].(map[string]any)["text"] != "  inspect 🚀\n " {
		t.Fatal("wire adapter changed user content block")
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

func TestStreamingPreservesInitialBlocksAndOptionalCallback(t *testing.T) {
	for _, kind := range []string{"text", "tool_use", "fragmented_tool"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				blocks := []string{
					`{"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":0}}}`,
				}
				switch kind {
				case "text":
					blocks = append(blocks, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"initial "}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"report"}}`)
				case "tool_use":
					blocks = append(blocks, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"spawn_agent","input":{"title":"worker","task":"write proof"}}}`)
				case "fragmented_tool":
					blocks = append(blocks, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"spawn_agent","input":{}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"title\":\"worker\","}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"task\":\"write proof\"}"}}`)
				}
				stop := "tool_use"
				if kind == "text" {
					stop = "end_turn"
				}
				blocks = append(blocks, `{"type":"content_block_stop","index":0}`, fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":9}}`, stop), `{"type":"message_stop"}`)
				for _, block := range blocks {
					var e struct {
						Type string `json:"type"`
					}
					_ = json.Unmarshal([]byte(block), &e)
					fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, block)
				}
			}))
			defer server.Close()
			result, err := NewAnthropicClient().StreamChatWithTools(context.Background(), llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderAnthropic, BaseURL: server.URL, APIKey: "test", Model: "test"}, []llmsvc.ChatMessage{{Role: "user", Content: "delegate"}}, nil, llmsvc.StreamCallbacks{})
			if err != nil {
				t.Fatal(err)
			}
			if kind == "text" {
				if result.AssistantMessage.Content != "initial report" {
					t.Fatalf("initial text was lost: %+v", result)
				}
			} else {
				if len(result.ToolCalls) != 1 {
					t.Fatalf("tool call missing: %+v", result)
				}
				var input map[string]string
				if err := json.Unmarshal([]byte(result.ToolCalls[0].Arguments), &input); err != nil || input["title"] != "worker" || input["task"] != "write proof" {
					t.Fatalf("tool input lost: %+v error=%v", result.ToolCalls, err)
				}
			}
		})
	}
}
