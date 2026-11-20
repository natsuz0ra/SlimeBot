package privacy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	antsvc "slimebot/internal/services/anthropic"
	"slimebot/internal/services/llm"
	oaisvc "slimebot/internal/services/openai"
)

func TestSDKRequestsContainOnlyMaskedContent(t *testing.T) {
	for _, protocol := range []string{"openai", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			s, _, ctx := setup(t)
			const secret = "sk-proj-OnlySyntheticKey9876543210abcdefgh"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), secret) {
					t.Error("raw sensitive value reached HTTP server")
				}
				p := placeholderRE.FindString(string(body))
				if p == "" {
					t.Error("outbound HTTP body has no placeholder")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if protocol == "openai" {
					if r.Header.Get("Authorization") != "Bearer provider-auth-key" {
						t.Error("authentication header masked")
					}
					data, _ := json.Marshal(map[string]any{"id": "synthetic", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "答复 " + p}, "finish_reason": "stop"}}})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
				} else {
					if r.Header.Get("X-Api-Key") != "provider-auth-key" {
						t.Error("authentication header masked")
					}
					fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_synthetic\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
					fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
					data, _ := json.Marshal(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "答复 " + p}})
					fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
					fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
				}
			}))
			defer upstream.Close()
			var next llm.Provider = oaisvc.NewOpenAIClient()
			if protocol == "anthropic" {
				next = antsvc.NewAnthropicClient()
			}
			var output strings.Builder
			_, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{Provider: protocol, BaseURL: upstream.URL + "/v1", APIKey: "provider-auth-key", Model: "test"}, []llm.ChatMessage{{Role: "user", Content: "Please use " + secret}}, nil, llm.StreamCallbacks{OnChunk: func(v string) error { output.WriteString(v); return nil }})
			if err != nil {
				t.Fatal(err)
			}
			if output.String() != "答复 "+secret {
				t.Fatalf("unexpected restored text: %q", output.String())
			}
		})
	}
}

func TestBypassAndDisabledRequestsRemainIntact(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	c.BypassProviderIDs = []string{"trusted"}
	_ = s.Update(ctx, c)
	called := 0
	next := fakeProvider{run: func(_ context.Context, _ llm.ModelRuntimeConfig, m []llm.ChatMessage, _ []llm.ToolDef, _ llm.StreamCallbacks) (*llm.StreamResult, error) {
		called++
		if m[0].Content != "DB_PASSWORD=TestSecret123" {
			t.Fatal("explicit bypass changed text")
		}
		return &llm.StreamResult{}, nil
	}}
	if _, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{ProviderID: "trusted"}, []llm.ChatMessage{{Content: "DB_PASSWORD=TestSecret123"}}, nil, llm.StreamCallbacks{}); err != nil {
		t.Fatal(err)
	}
	c.Enabled = false
	_ = s.Update(ctx, c)
	if _, err := s.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Content: "DB_PASSWORD=TestSecret123"}}, nil, llm.StreamCallbacks{}); err != nil {
		t.Fatal(err)
	}
	if called != 2 {
		t.Fatal("provider not called")
	}
}
