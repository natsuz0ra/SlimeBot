package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearchToolSearchSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"query":"Who is Leo Messi?",
			"answer":"Messi is an Argentine footballer.",
			"results":[
				{"title":"Britannica","url":"https://www.britannica.com/facts/Lionel-Messi","content":"summary text","score":0.88}
			]
		}`))
	}))
	defer server.Close()

	tool := newWebSearchTool(server.URL, server.Client(), func() string { return "test-key" })
	result, err := tool.Execute("search", map[string]string{"query": "Who is Leo Messi?"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Error != "" {
		t.Fatalf("expected empty result error, got %s", result.Error)
	}
	if !strings.Contains(result.Output, "Messi is an Argentine footballer.") {
		t.Fatalf("expected output to contain answer, got %s", result.Output)
	}
	if !strings.Contains(result.Output, "https://www.britannica.com/facts/Lionel-Messi") {
		t.Fatalf("expected output to contain source URL, got %s", result.Output)
	}
}

func TestWebSearchToolMissingAPIKey(t *testing.T) {
	tool := newWebSearchTool("https://api.tavily.com", &http.Client{}, func() string { return "" })
	_, err := tool.Execute("search", map[string]string{"query": "test"})
	if err == nil {
		t.Fatal("expected error when API key is empty")
	}
	if !strings.Contains(err.Error(), "WEB_SEARCH_API_KEY") {
		t.Fatalf("expected env key hint in error, got %v", err)
	}
}

func TestWebSearchToolUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":{"error":"Invalid topic. Must be 'general' or 'news'."}}`))
	}))
	defer server.Close()

	tool := newWebSearchTool(server.URL, server.Client(), func() string { return "test-key" })
	result, err := tool.Execute("search", map[string]string{"query": "test"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(result.Error, "Invalid topic") {
		t.Fatalf("expected upstream detail error, got %s", result.Error)
	}
}
