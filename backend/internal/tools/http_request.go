package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	httpRequestTimeout   = 30 * time.Second
	httpMaxResponseBytes = 128 * 1024 // 128KB
)

type httpRequestTool struct {
	client *http.Client
}

func init() {
	Register(&httpRequestTool{
		client: &http.Client{Timeout: httpRequestTimeout},
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
		return nil, fmt.Errorf("http_request 工具不支持命令: %s", command)
	}
}

func (h *httpRequestTool) request(params map[string]string) (*ExecuteResult, error) {
	method := strings.ToUpper(strings.TrimSpace(params["method"]))
	if method == "" {
		return nil, fmt.Errorf("参数 method 不能为空")
	}

	rawURL := strings.TrimSpace(params["url"])
	if rawURL == "" {
		return nil, fmt.Errorf("参数 url 不能为空")
	}

	var bodyReader io.Reader
	if body := strings.TrimSpace(params["body"]); body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	if headersStr := strings.TrimSpace(params["headers"]); headersStr != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
			return nil, fmt.Errorf("headers 格式错误，需要 JSON 对象: %w", err)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("请求失败: %s", err.Error())}, nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, httpMaxResponseBytes))
	if err != nil {
		return &ExecuteResult{Error: fmt.Sprintf("读取响应体失败: %s", err.Error())}, nil
	}

	respHeaders := make(map[string]string)
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}
	headersJSON, _ := json.Marshal(respHeaders)

	result := fmt.Sprintf("状态码: %d\n响应头: %s\n响应体:\n%s", resp.StatusCode, string(headersJSON), string(bodyBytes))
	return &ExecuteResult{Output: result}, nil
}
