package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"slimebot/backend/internal/consts"
)

type httpRequestTool struct {
	client *http.Client
}

func init() {
	Register(&httpRequestTool{
		client: &http.Client{Timeout: consts.HTTPRequestTimeout},
	})
}

func (h *httpRequestTool) Name() string { return "http_request" }

func (h *httpRequestTool) Description() string {
	return "HTTP 请求工具，可以向指定 URL 发送 HTTP 请求并返回响应状态码、响应头和响应体"
}

func (h *httpRequestTool) Commands() []Command {
	return []Command{
		{
			Name:        "request",
			Description: "发送一个 HTTP 请求，返回响应的状态码、headers 和 body",
			Params: []CommandParam{
				{Name: "method", Required: true, Description: "HTTP 方法，如 GET、POST、PUT、DELETE 等", Example: "GET"},
				{Name: "url", Required: true, Description: "请求的完整 URL", Example: "https://api.example.com/data"},
				{Name: "headers", Required: false, Description: "请求头，JSON 对象格式，key 为 header 名，value 为 header 值", Example: `{"Content-Type":"application/json","Authorization":"Bearer xxx"}`},
				{Name: "body", Required: false, Description: "请求体内容，仅 POST/PUT/PATCH 等方法时有效", Example: `{"key":"value"}`},
			},
		},
	}
}

func (h *httpRequestTool) Execute(command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "request":
		return h.request(params)
	default:
		return nil, fmt.Errorf("http_request tool does not support command: %s", command)
	}
}

func (h *httpRequestTool) request(params map[string]string) (*ExecuteResult, error) {
	method := strings.ToUpper(strings.TrimSpace(params["method"]))
	if method == "" {
		return nil, fmt.Errorf("method is required.")
	}

	rawURL := strings.TrimSpace(params["url"])
	if rawURL == "" {
		return nil, fmt.Errorf("url is required.")
	}

	var bodyReader io.Reader
	if body := strings.TrimSpace(params["body"]); body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	if headersStr := strings.TrimSpace(params["headers"]); headersStr != "" {
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

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, consts.HTTPMaxResponseBytes))
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("Failed to read response body: %s.", err.Error())}, nil
	}

	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}
	headersJSON, _ := json.Marshal(respHeaders)

	result := fmt.Sprintf("状态码: %d\n响应头: %s\n响应体:\n%s", resp.StatusCode, string(headersJSON), string(bodyBytes))
	return &ExecuteResult{Output: result}, nil
}
