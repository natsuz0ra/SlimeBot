package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ServerConfig 描述单个 MCP 服务的连接配置。
type ServerConfig struct {
	Transport      string            `json:"transport"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	URL            string            `json:"url"`
	Headers        map[string]string `json:"headers"`
	Timeout        int               `json:"timeout"`
	SSEReadTimeout int               `json:"sse_read_timeout"`
}

// ParseAndValidateConfig 解析并校验 MCP 配置，返回可直接使用的标准化结果。
func ParseAndValidateConfig(raw string) (*ServerConfig, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return nil, fmt.Errorf("config is required.")
	}

	var cfg ServerConfig
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("config JSON is invalid: %w", err)
	}

	transport := strings.TrimSpace(cfg.Transport)
	if transport == "" {
		// 与历史行为保持一致：未显式指定时默认走 stdio。
		transport = "stdio"
		cfg.Transport = transport
	}

	// 不同 transport 的必填字段不同，按协议类型分别校验。
	switch transport {
	case "stdio":
		if strings.TrimSpace(cfg.Command) == "" {
			return nil, fmt.Errorf("stdio config requires command.")
		}
	case "streamable_http", "sse":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf("%s config requires url.", transport)
		}
	default:
		return nil, fmt.Errorf("unsupported transport: %s", transport)
	}

	return &cfg, nil
}
