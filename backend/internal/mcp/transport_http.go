package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type httpClient struct {
	url        string
	headers    map[string]string
	httpClient *http.Client
	id         int64
}

func newHTTPClient(cfg *ServerConfig) Client {
	timeoutSec := cfg.Timeout
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &httpClient{
		url:     strings.TrimSpace(cfg.URL),
		headers: cfg.Headers,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
	}
}

func (c *httpClient) nextID() int64 {
	return atomic.AddInt64(&c.id, 1)
}

func (c *httpClient) postRPC(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.nextID(),
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mcp http status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var rpc map[string]any
	if err := json.Unmarshal(raw, &rpc); err != nil {
		return nil, fmt.Errorf("mcp http response 非法 JSON: %w", err)
	}
	if errObj, ok := rpc["error"]; ok && errObj != nil {
		return nil, fmt.Errorf("mcp rpc error: %v", errObj)
	}
	result, _ := rpc["result"].(map[string]any)
	return result, nil
}

func (c *httpClient) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.postRPC(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	toolItems, _ := result["tools"].([]any)
	tools := make([]Tool, 0, len(toolItems))
	for _, item := range toolItems {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := obj["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		description, _ := obj["description"].(string)
		inputSchema, _ := obj["inputSchema"].(map[string]any)
		tools = append(tools, Tool{
			Name:        name,
			Description: description,
			InputSchema: inputSchema,
		})
	}
	return tools, nil
}

func (c *httpClient) CallTool(ctx context.Context, name string, arguments map[string]any) (*CallResult, error) {
	result, err := c.postRPC(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	return parseCallResult(result), nil
}

func (c *httpClient) Close() error { return nil }

func parseCallResult(result map[string]any) *CallResult {
	var out strings.Builder
	if contents, ok := result["content"].([]any); ok {
		for _, c := range contents {
			item, ok := c.(map[string]any)
			if !ok {
				continue
			}
			text, _ := item["text"].(string)
			if text == "" {
				continue
			}
			if out.Len() > 0 {
				out.WriteString("\n")
			}
			out.WriteString(text)
		}
	}

	callErr := ""
	if isError, _ := result["isError"].(bool); isError {
		callErr = out.String()
	}
	return &CallResult{
		Output: out.String(),
		Error:  callErr,
	}
}
