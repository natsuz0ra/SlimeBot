package tools

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"slimebot/internal/constants"
	sandboxpolicy "slimebot/internal/sandbox"
)

type webExtractTool struct {
	client *http.Client
}

func init() {
	Register(&webExtractTool{client: &http.Client{Timeout: constants.HTTPRequestTimeout}})
}

func (w *webExtractTool) Name() string { return "web_extract" }

func (w *webExtractTool) Description() string {
	return "Fetch a specific web page URL and extract readable title and text."
}

func (w *webExtractTool) Commands() []Command {
	return []Command{{
		Name:        "extract",
		Description: "Extract readable text from one http/https URL. Use web_search for discovery and this tool for known pages.",
		Params: []CommandParam{
			{Name: "url", Required: true, Description: "HTTP or HTTPS URL to extract.", Example: "https://example.com/page"},
		},
	}}
}

func (w *webExtractTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "extract":
		return w.extract(ctx, params)
	default:
		return nil, fmt.Errorf("web_extract tool does not support command: %s", command)
	}
}

func (w *webExtractTool) extract(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	rawURL := paramStringTrim(params, "url")
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("url must be absolute")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("url must be absolute")
	}
	if policy, ok := sandboxpolicy.FromContext(ctx); ok {
		if err := policy.CheckNetworkURL(rawURL); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "SlimeBot web_extract")
	client := w.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("web_extract request failed: %s.", err.Error())}, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, constants.HTTPMaxResponseBytes))
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("Failed to read web_extract response: %s.", err.Error())}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ExecuteResult{Error: fmt.Sprintf("web_extract request failed (status %d).", resp.StatusCode)}, nil
	}
	title, text := extractReadableHTML(string(body))
	if text == "" {
		text = strings.TrimSpace(string(body))
	}
	if len([]rune(text)) > 6000 {
		text = string([]rune(text)[:6000]) + "..."
	}
	var out strings.Builder
	out.WriteString("URL: " + rawURL + "\n")
	if title != "" {
		out.WriteString("Title: " + title + "\n")
	}
	out.WriteString("Content:\n" + text)
	return &ExecuteResult{Output: strings.TrimSpace(out.String())}, nil
}

func extractReadableHTML(input string) (string, string) {
	title := ""
	if m := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`).FindStringSubmatch(input); len(m) > 1 {
		title = cleanHTMLText(m[1])
	}
	body := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(input, " ")
	body = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(body, " ")
	body = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(body, " ")
	return title, cleanHTMLText(body)
}

func cleanHTMLText(input string) string {
	text := html.UnescapeString(input)
	text = strings.Join(strings.Fields(text), " ")
	return strings.TrimSpace(text)
}
