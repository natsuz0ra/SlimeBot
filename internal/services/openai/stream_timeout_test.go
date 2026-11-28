package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	llmsvc "slimebot/internal/services/llm"
)

func TestLongLivedStreamHasNoWholeRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"long\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		// Model thinking/streaming time is bounded by the turn context, not an
		// http.Client whole-response deadline.
		time.Sleep(150 * time.Millisecond)
		fmt.Fprint(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" stream\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewOpenAIClient()
	if client.Client.Timeout != 0 {
		t.Fatalf("whole-request timeout = %v, want 0", client.Client.Timeout)
	}
	result, err := client.StreamChatWithTools(
		context.Background(),
		llmsvc.ModelRuntimeConfig{Provider: llmsvc.ProviderOpenAI, BaseURL: server.URL, APIKey: "key", Model: "custom"},
		[]llmsvc.ChatMessage{{Role: "user", Content: "test"}},
		nil,
		llmsvc.StreamCallbacks{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssistantMessage.Content != "long stream" {
		t.Fatalf("stream content = %q, want %q", result.AssistantMessage.Content, "long stream")
	}
}
