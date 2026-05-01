package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"slimebot/internal/constants"
)

type httpRequestTool struct {
	client *http.Client
}

func init() {
	Register(&httpRequestTool{
		client: &http.Client{Timeout: constants.HTTPRequestTimeout},
	})
}

func (h *httpRequestTool) Name() string { return "http_request" }

func (h *httpRequestTool) Description() string {
	return "Send HTTP requests to a URL and return status, headers, and body."
}

func (h *httpRequestTool) Commands() []Command {
	return []Command{
		{
			Name:        "request",
			Description: "Send an HTTP request and return status code, headers, and body.",
			Params: []CommandParam{
				{Name: "method", Required: true, Description: "HTTP method, such as GET, POST, PUT, DELETE, etc.", Example: "GET"},
				{Name: "url", Required: true, Description: "Full request URL.", Example: "https://api.example.com/data"},
				{Name: "headers", Required: false, Description: "Request headers as a JSON object where key is header name and value is header value.", Example: `{"Content-Type":"application/json","Authorization":"Bearer xxx"}`},
				{Name: "body", Required: false, Description: "Request body content, used for methods like POST/PUT/PATCH.", Example: `{"key":"value"}`},
			},
		},
	}
}

func (h *httpRequestTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "request":
		return h.request(ctx, params)
	default:
		return nil, fmt.Errorf("http_request tool does not support command: %s", command)
	}
}

func (h *httpRequestTool) request(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	method := strings.ToUpper(paramStringTrim(params, "method"))
	if method == "" {
		return nil, fmt.Errorf("method is required.")
	}

	rawURL := paramStringTrim(params, "url")
	if rawURL == "" {
		return nil, fmt.Errorf("url is required.")
	}

	var bodyReader io.Reader
	if body := paramStringTrim(params, "body"); body != "" {
		bodyReader = strings.NewReader(body)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req = req.WithContext(ctx)

	if headersStr := paramStringTrim(params, "headers"); headersStr != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
			return nil, fmt.Errorf("invalid headers format; expected a JSON object: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("Request failed: %s.", err.Error())}, nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, constants.HTTPMaxResponseBytes))
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("Failed to read response body: %s.", err.Error())}, nil
	}

	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}
	headersJSON, _ := json.Marshal(respHeaders)

	result := fmt.Sprintf("Status: %d\nHeaders: %s\nBody:\n%s", resp.StatusCode, string(headersJSON), string(bodyBytes))
	return &ExecuteResult{Output: result}, nil
}
