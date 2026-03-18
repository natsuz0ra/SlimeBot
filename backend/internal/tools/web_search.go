package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"slimebot/backend/internal/consts"
)

type webSearchTool struct {
	baseURL   string
	client    *http.Client
	getAPIKey func() string
}

type tavilySearchRequest struct {
	Query string `json:"query"`
}

type tavilySearchResponse struct {
	Query   string `json:"query"`
	Answer  string `json:"answer"`
	Results []struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	} `json:"results"`
}

type tavilyErrorResponse struct {
	Detail any `json:"detail"`
}

func init() {
	Register(newWebSearchTool(
		consts.WebSearchBaseURL,
		&http.Client{Timeout: consts.WebSearchTimeout},
		func() string { return os.Getenv("WEB_SEARCH_API_KEY") },
	))
}

func newWebSearchTool(baseURL string, client *http.Client, getAPIKey func() string) *webSearchTool {
	return &webSearchTool{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:    client,
		getAPIKey: getAPIKey,
	}
}

func (w *webSearchTool) Name() string { return "web_search" }

func (w *webSearchTool) Description() string {
	return "Search the web for recent public information and return answer summaries with source links."
}

func (w *webSearchTool) Commands() []Command {
	return []Command{
		{
			Name:        "search",
			Description: "Run a web search and return structured results, including upstream/validation/auth errors when they occur.",
			Params: []CommandParam{
				{
					Name:        "query",
					Required:    true,
					Description: "Search query. Prefer complete and specific questions over empty or vague keywords.",
					Example:     "Who is Leo Messi?",
				},
			},
		},
	}
}

func (w *webSearchTool) Execute(command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "search":
		return w.search(params)
	default:
		return nil, fmt.Errorf("web_search tool does not support command: %s", command)
	}
}

func (w *webSearchTool) search(params map[string]string) (*ExecuteResult, error) {
	query := strings.TrimSpace(params["query"])
	if query == "" {
		return nil, fmt.Errorf("query is required.")
	}

	apiKey := strings.TrimSpace(w.getAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("WEB_SEARCH_API_KEY is not configured; web_search is unavailable.")
	}

	reqBody, err := json.Marshal(tavilySearchRequest{Query: query})
	if err != nil {
		return nil, fmt.Errorf("failed to build request body: %w", err)
	}

	endpoint := w.baseURL + "/search"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("web_search request failed: %s.", err.Error())}, nil
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, consts.WebSearchMaxResponseSize))
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("Failed to read web_search response: %s.", err.Error())}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &ExecuteResult{Error: parseTavilyError(resp.StatusCode, rawBody)}, nil
	}

	var data tavilySearchResponse
	if err := json.Unmarshal(rawBody, &data); err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("Failed to parse web_search response: %s.", err.Error())}, nil
	}

	return &ExecuteResult{
		Output: formatTavilyOutput(data),
	}, nil
}

func parseTavilyError(statusCode int, rawBody []byte) string {
	var er tavilyErrorResponse
	if err := json.Unmarshal(rawBody, &er); err == nil && er.Detail != nil {
		if msg := extractDetailError(er.Detail); msg != "" {
			return fmt.Sprintf("web_search request failed (status %d): %s", statusCode, msg)
		}
	}
	body := strings.TrimSpace(string(rawBody))
	if body == "" {
		body = "empty response body"
	}
	return fmt.Sprintf("web_search request failed (status %d): %s", statusCode, body)
}

func extractDetailError(detail any) string {
	switch v := detail.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if e, ok := v["error"]; ok {
			if s, ok := e.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func formatTavilyOutput(data tavilySearchResponse) string {
	var b strings.Builder
	if q := strings.TrimSpace(data.Query); q != "" {
		b.WriteString("Query: " + q + "\n")
	}
	if a := strings.TrimSpace(data.Answer); a != "" {
		b.WriteString("Answer:\n")
		b.WriteString(a + "\n")
	}

	if len(data.Results) == 0 {
		return strings.TrimSpace(b.String())
	}

	b.WriteString("Sources:\n")
	limit := len(data.Results)
	if limit > consts.WebSearchMaxSources {
		limit = consts.WebSearchMaxSources
	}
	for i := 0; i < limit; i++ {
		item := data.Results[i]
		title := strings.TrimSpace(item.Title)
		url := strings.TrimSpace(item.URL)
		content := truncateRunes(strings.TrimSpace(item.Content), consts.WebSearchMaxContentRunes)

		if title == "" {
			title = "Untitled source"
		}
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, title))
		if url != "" {
			b.WriteString("   URL: " + url + "\n")
		}
		if content != "" {
			b.WriteString("   Snippet: " + content + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func truncateRunes(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max]) + "..."
}
