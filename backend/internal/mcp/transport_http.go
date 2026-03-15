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

// newHTTPClient 基于服务配置创建 HTTP 传输客户端。
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

// postRPC 发送 JSON-RPC 请求，并将响应中的 result 提取为 map 返回。
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
	// MCP 规范中的 error 字段优先级高于 result，统一在这里抛出。
	if errObj, ok := rpc["error"]; ok && errObj != nil {
		return nil, fmt.Errorf("mcp rpc error: %v", errObj)
	}
	result, _ := rpc["result"].(map[string]any)
	return result, nil
}

// ListTools 获取 MCP 服务工具列表并转为内部统一结构。
func (c *httpClient) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.postRPC(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseTools(result), nil
}

// CallTool 调用指定工具并返回标准化调用结果。
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

// Close 关闭 HTTP 客户端。HTTP 为无状态连接，此处无需额外资源回收。
func (c *httpClient) Close() error { return nil }
