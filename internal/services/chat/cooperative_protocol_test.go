package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	anthropic "slimebot/internal/services/anthropic"
	llm "slimebot/internal/services/llm"
	openai "slimebot/internal/services/openai"
)

// Exercise the actual HTTP adapters with cooperative tool schemas, fragmented
// parallel calls, tool results and a host-delivered continuation on the next step.
func TestCooperativeProviderWireRoundTrip(t *testing.T) {
	for _, protocol := range []string{llm.ProviderOpenAI, llm.ProviderDeepSeek, llm.ProviderAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				n := requests.Add(1)
				if protocol == llm.ProviderAnthropic || protocol == llm.ProviderDeepSeek {
					thinking, _ := body["thinking"].(map[string]any)
					if thinking["type"] != "disabled" {
						t.Error("thinking off was not explicit on the wire")
					}
				} else if _, ok := body["thinking"]; ok {
					t.Error("DeepSeek thinking extension leaked into generic OpenAI protocol")
				}
				encoded, _ := json.Marshal(body)
				if strings.Contains(string(encoded), "sourceKind") || strings.Contains(string(encoded), "sourceInboxId") {
					t.Error("host routing fields leaked into provider request")
				}
				tools, ok := body["tools"].([]any)
				if !ok || len(tools) != len(cooperativeDefs()) {
					t.Error("cooperative tools missing from wire")
				}
				for _, raw := range tools {
					tool := raw.(map[string]any)
					var schema map[string]any
					if protocol == llm.ProviderAnthropic {
						schema, _ = tool["input_schema"].(map[string]any)
					} else {
						schema, _ = tool["function"].(map[string]any)["parameters"].(map[string]any)
					}
					if schema["type"] != "object" || schema["additionalProperties"] != false {
						t.Errorf("schema constraints lost: %+v", schema)
					}
					if required, exists := schema["required"]; exists {
						if _, ok := required.([]any); !ok {
							t.Errorf("invalid required field: %+v", schema)
						}
					}
				}
				if n == 2 {
					if !strings.Contains(string(encoded), "child accepted") || !strings.Contains(string(encoded), "follow up") {
						t.Error("results or continuation lost")
					}
					if !strings.Contains(string(encoded), "call_spawn") || !strings.Contains(string(encoded), "call_wait") {
						t.Error("tool IDs lost")
					}
					if protocol == llm.ProviderAnthropic {
						if !strings.Contains(string(encoded), `"signature":"sig-1"`) || strings.Count(string(encoded), `"type":"tool_result"`) != 2 {
							t.Error("thinking signature or grouped tool results lost")
						}
					} else if !strings.Contains(string(encoded), `"reasoning_content":"reason"`) {
						t.Error("compatible reasoning continuation lost")
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if protocol == llm.ProviderAnthropic {
					if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "test" {
						t.Error("wrong Anthropic endpoint/auth")
					}
					writeAnthropicCooperativeResponse(w, n == 1)
				} else {
					if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer test" {
						t.Error("wrong Chat Completions endpoint/auth")
					}
					writeOpenAICooperativeResponse(w, n == 1)
				}
			}))
			defer server.Close()
			factory := llm.NewFactory(openai.NewOpenAIClient())
			factory.Register(llm.ProviderAnthropic, anthropic.NewAnthropicClient())
			provider := factory.GetProvider(protocol)
			config := llm.ModelRuntimeConfig{Provider: protocol, BaseURL: server.URL, APIKey: "test", Model: "test", ContextSize: 8192, MaxOutputTokens: 256, Purpose: "chat"}
			messages := []llm.ChatMessage{{Role: "system", Content: "cooperate"}, {Role: "user", Content: "delegate", SourceKind: "delegated_task", SourceInboxID: "inbox-initial"}}
			var settled atomic.Int32
			ctx := llm.WithRequestObserver(context.Background(), func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef) (func(*llm.StreamResult, error) error, error) {
				return func(_ *llm.StreamResult, err error) error { settled.Add(1); return err }, nil
			})
			first, err := provider.StreamChatWithTools(ctx, config, messages, cooperativeDefs(), llm.StreamCallbacks{})
			if err != nil {
				t.Fatal(err)
			}
			if first.Type != llm.StreamResultToolCalls || len(first.ToolCalls) != 2 {
				t.Fatalf("parallel tools lost: %+v", first)
			}
			for i, call := range first.ToolCalls {
				var params map[string]any
				if err := json.Unmarshal([]byte(call.Arguments), &params); err != nil {
					t.Fatal(err)
				}
				if i == 0 && (call.Name != "spawn_agent" || params["task"] != "work") {
					t.Fatalf("fragmented spawn lost: %+v", call)
				}
				if i == 1 && (call.Name != "wait_agents" || params["timeout_ms"] != float64(0)) {
					t.Fatalf("wait call lost: %+v", call)
				}
			}
			messages = append(messages, first.AssistantMessage, llm.ChatMessage{Role: "tool", ToolCallID: first.ToolCalls[0].ID, Content: "child accepted"}, llm.ChatMessage{Role: "tool", ToolCallID: first.ToolCalls[1].ID, Content: "child finished"}, llm.ChatMessage{Role: "user", Content: "follow up", SourceKind: "human_input", SourceInboxID: "inbox-followup"})
			final, err := provider.StreamChatWithTools(ctx, config, messages, cooperativeDefs(), llm.StreamCallbacks{})
			if err != nil {
				t.Fatal(err)
			}
			if final.Type != llm.StreamResultText || final.AssistantMessage.Content != "completed" || final.TokenUsage == nil || final.TokenUsage.OutputTokens != 12 {
				t.Fatalf("final answer/usage lost: %+v", final)
			}
			if requests.Load() != 2 || settled.Load() != 2 {
				t.Fatal("request settlement did not cover both steps")
			}
		})
	}
}

func writeOpenAICooperativeResponse(w http.ResponseWriter, tools bool) {
	chunk := func(delta map[string]any, finish any) {
		x := map[string]any{"id": "test", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
		b, _ := json.Marshal(x)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	if tools {
		chunk(map[string]any{"role": "assistant", "reasoning_content": "reason", "tool_calls": []any{
			map[string]any{"index": 0, "id": "call_spawn", "type": "function", "function": map[string]any{"name": "spawn_agent", "arguments": `{"title":"tiny",`}},
			map[string]any{"index": 1, "id": "call_wait", "type": "function", "function": map[string]any{"name": "wait_agents", "arguments": `{"timeout_`}},
		}}, nil)
		chunk(map[string]any{"tool_calls": []any{
			map[string]any{"index": 1, "function": map[string]any{"arguments": `ms":0}`}},
			map[string]any{"index": 0, "function": map[string]any{"arguments": `"task":"work"}`}},
		}}, "tool_calls")
	} else {
		chunk(map[string]any{"role": "assistant", "content": "completed"}, "stop")
	}
	fmt.Fprint(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":12,\"total_tokens\":112}}\n\ndata: [DONE]\n\n")
}

func writeAnthropicCooperativeResponse(w http.ResponseWriter, tools bool) {
	event := func(name string, data map[string]any) {
		data["type"] = name
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
	}
	event("message_start", map[string]any{"message": map[string]any{"usage": map[string]any{"input_tokens": 100, "output_tokens": 0}}})
	stop := "end_turn"
	if tools {
		stop = "tool_use"
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "thinking", "thinking": "reason", "signature": ""}})
		event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "signature_delta", "signature": "sig-1"}})
		event("content_block_stop", map[string]any{"index": 0})
		for i, name := range []string{"spawn_agent", "wait_agents"} {
			id, args := "call_spawn", `{"title":"tiny","task":"work"}`
			if i == 1 {
				id, args = "call_wait", `{"timeout_ms":0}`
			}
			event("content_block_start", map[string]any{"index": i + 1, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
			for _, part := range []string{args[:len(args)/2], args[len(args)/2:]} {
				event("content_block_delta", map[string]any{"index": i + 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": part}})
			}
			event("content_block_stop", map[string]any{"index": i + 1})
		}
	} else {
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": "completed"}})
		event("content_block_stop", map[string]any{"index": 0})
	}
	event("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stop}, "usage": map[string]any{"output_tokens": 12}})
	event("message_stop", map[string]any{})
}

func TestCooperativeProviderRejectsTruncatedStreams(t *testing.T) {
	for _, protocol := range []string{llm.ProviderOpenAI, llm.ProviderDeepSeek, llm.ProviderAnthropic} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if protocol == llm.ProviderAnthropic {
					fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call\",\"name\":\"spawn_agent\",\"input\":{}}}\n\n")
				} else {
					fmt.Fprint(w, "data: {\"id\":\"partial\",\"object\":\"chat.completion.chunk\",\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"spawn_agent\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n")
				}
			}))
			defer server.Close()
			factory := llm.NewFactory(openai.NewOpenAIClient())
			factory.Register(llm.ProviderAnthropic, anthropic.NewAnthropicClient())
			result, err := factory.GetProvider(protocol).StreamChatWithTools(context.Background(), llm.ModelRuntimeConfig{Provider: protocol, BaseURL: server.URL, APIKey: "test", Model: "test"}, []llm.ChatMessage{{Role: "user", Content: "delegate"}}, cooperativeDefs(), llm.StreamCallbacks{})
			if err == nil || result != nil {
				t.Fatalf("truncated stream exposed executable calls: result=%+v error=%v", result, err)
			}
		})
	}
}
