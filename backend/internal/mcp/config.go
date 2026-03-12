package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ServerConfig struct {
	Transport      string            `json:"transport"`
	Command        string            `json:"command"`
	Args           []string          `json:"args"`
	URL            string            `json:"url"`
	Headers        map[string]string `json:"headers"`
	Timeout        int               `json:"timeout"`
	SSEReadTimeout int               `json:"sse_read_timeout"`
}

func ParseAndValidateConfig(raw string) (*ServerConfig, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return nil, fmt.Errorf("config 不能为空")
	}

	var cfg ServerConfig
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("config JSON 解析失败: %w", err)
	}

	transport := strings.TrimSpace(cfg.Transport)
	if transport == "" {
		transport = "stdio"
		cfg.Transport = transport
	}

	switch transport {
	case "stdio":
		if strings.TrimSpace(cfg.Command) == "" {
			return nil, fmt.Errorf("stdio 配置必须提供 command")
		}
	case "streamable_http", "sse":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf("%s 配置必须提供 url", transport)
		}
	default:
		return nil, fmt.Errorf("不支持的 transport: %s", transport)
	}

	return &cfg, nil
}
