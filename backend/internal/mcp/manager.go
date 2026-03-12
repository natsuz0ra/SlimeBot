package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"corner/backend/internal/models"
)

type ToolMeta struct {
	FuncName    string
	ServerAlias string
	ToolName    string
}

type Manager struct {
	mu      sync.Mutex
	clients map[string]*managedClient
}

type managedClient struct {
	configID string
	raw      string
	alias    string
	client   Client
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*managedClient),
	}
}

func (m *Manager) LoadTools(ctx context.Context, configs []models.MCPConfig) ([]ToolMeta, []map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	alive := make(map[string]bool, len(configs))
	var metas []ToolMeta
	var defs []map[string]any

	for _, item := range configs {
		if !item.IsEnabled {
			continue
		}
		alive[item.ID] = true
		entry, err := m.ensureClientLocked(item)
		if err != nil {
			return nil, nil, fmt.Errorf("初始化 MCP 服务失败(%s): %w", item.Name, err)
		}
		tools, err := entry.client.ListTools(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("获取 MCP 工具失败(%s): %w", item.Name, err)
		}

		for _, tool := range tools {
			commandAlias := sanitizeToken(tool.Name)
			funcName := entry.alias + "__" + commandAlias
			inputSchema := tool.InputSchema
			if inputSchema == nil {
				inputSchema = map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				}
			}
			defs = append(defs, map[string]any{
				"name":        funcName,
				"description": fmt.Sprintf("[mcp:%s] %s", item.Name, strings.TrimSpace(tool.Description)),
				"parameters":  inputSchema,
			})
			metas = append(metas, ToolMeta{
				FuncName:    funcName,
				ServerAlias: entry.alias,
				ToolName:    tool.Name,
			})
		}
	}

	for id, entry := range m.clients {
		if alive[id] {
			continue
		}
		_ = entry.client.Close()
		delete(m.clients, id)
	}

	return metas, defs, nil
}

func (m *Manager) ensureClientLocked(item models.MCPConfig) (*managedClient, error) {
	existing, ok := m.clients[item.ID]
	if ok && existing.raw == item.Config {
		return existing, nil
	}
	if ok {
		_ = existing.client.Close()
		delete(m.clients, item.ID)
	}

	cfg, err := ParseAndValidateConfig(item.Config)
	if err != nil {
		return nil, err
	}
	var cli Client
	switch cfg.Transport {
	case "stdio":
		cli, err = newStdioClient(cfg)
	case "streamable_http", "sse":
		cli = newHTTPClient(cfg)
	default:
		err = fmt.Errorf("不支持的 transport: %s", cfg.Transport)
	}
	if err != nil {
		return nil, err
	}

	alias := "mcp_" + sanitizeToken(item.ID)
	entry := &managedClient{
		configID: item.ID,
		raw:      item.Config,
		alias:    alias,
		client:   cli,
	}
	m.clients[item.ID] = entry
	return entry, nil
}

func (m *Manager) Execute(ctx context.Context, configs []models.MCPConfig, serverAlias, toolName string, arguments map[string]any) (*CallResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var target models.MCPConfig
	found := false
	for _, item := range configs {
		if !item.IsEnabled {
			continue
		}
		if "mcp_"+sanitizeToken(item.ID) == serverAlias {
			target = item
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("MCP 服务不存在或未启用: %s", serverAlias)
	}

	entry, err := m.ensureClientLocked(target)
	if err != nil {
		return nil, err
	}
	return entry.client.CallTool(ctx, toolName, arguments)
}

func sanitizeToken(input string) string {
	if strings.TrimSpace(input) == "" {
		return "x"
	}
	var b strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "x"
	}
	return out
}
