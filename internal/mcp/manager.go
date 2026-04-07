package mcp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
)

// ToolMeta 记录函数调用定义与 MCP 真实工具之间的映射关系
type ToolMeta struct {
	FuncName    string
	ServerAlias string
	ToolName    string
}

// Manager MCP 管理器：负责管理 MCP 客户端实例，提供工具加载与执行能力
type Manager struct {
	mu      sync.Mutex
	clients map[string]*managedClient
}

type managedClient struct {
	configID string
	raw      string
	alias    string
	client   Client
	clientMu sync.Mutex
}

// NewManager 创建一个新的 MCP 管理器实例
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*managedClient),
	}
}

// CloseAll 关闭并清空所有已托管的 MCP 客户端连接
func (m *Manager) CloseAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	entries := make([]*managedClient, 0, len(m.clients))
	for _, e := range m.clients {
		entries = append(entries, e)
	}
	m.clients = make(map[string]*managedClient)
	m.mu.Unlock()
	for _, entry := range entries {
		entry.clientMu.Lock()
		_ = entry.client.Close()
		entry.clientMu.Unlock()
	}
}

// LoadTools 加载当前启用服务的工具定义，返回函数元数据与 OpenAI 工具描述
func (m *Manager) LoadTools(ctx context.Context, configs []domain.MCPConfig) ([]ToolMeta, []map[string]any, error) {
	alive := make(map[string]bool, len(configs))
	var metas []ToolMeta
	var defs []map[string]any

	type target struct {
		item  domain.MCPConfig
		entry *managedClient
	}
	var targets []target
	var toClose []*managedClient

	m.mu.Lock()
	for _, item := range configs {
		if !item.IsEnabled {
			continue
		}
		alive[item.ID] = true
		entry, err := m.ensureClientLocked(item)
		if err != nil {
			m.mu.Unlock()
			return nil, nil, fmt.Errorf("failed to initialize MCP service (%s): %w", item.Name, err)
		}
		targets = append(targets, target{item: item, entry: entry})
	}

	for id, entry := range m.clients {
		if alive[id] {
			continue
		}
		// 配置被禁用或删除后主动回收连接，避免保留失活客户端。
		toClose = append(toClose, entry)
		delete(m.clients, id)
	}
	m.mu.Unlock()

	defer func() {
		for _, entry := range toClose {
			entry.clientMu.Lock()
			_ = entry.client.Close()
			entry.clientMu.Unlock()
		}
	}()

	type listResult struct {
		metas []ToolMeta
		defs  []map[string]any
		err   error
	}
	results := make([]listResult, len(targets))
	var wg sync.WaitGroup
	parallelStart := time.Now()
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t target) {
			defer wg.Done()
			entry := t.entry
			entry.clientMu.Lock()
			tools, err := entry.client.ListTools(ctx)
			entry.clientMu.Unlock()
			if err != nil {
				results[i].err = fmt.Errorf("failed to load MCP tools (%s): %w", t.item.Name, err)
				return
			}
			var lm []ToolMeta
			var ld []map[string]any
			for _, tool := range tools {
				funcName := buildMCPFuncName(entry.alias, tool.Name)
				inputSchema := tool.InputSchema
				if inputSchema == nil {
					inputSchema = map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					}
				}
				ld = append(ld, map[string]any{
					"name":        funcName,
					"description": fmt.Sprintf("[mcp:%s] %s", t.item.Name, strings.TrimSpace(tool.Description)),
					"parameters":  inputSchema,
				})
				lm = append(lm, ToolMeta{
					FuncName:    funcName,
					ServerAlias: entry.alias,
					ToolName:    tool.Name,
				})
			}
			results[i].metas = lm
			results[i].defs = ld
		}(i, t)
	}
	wg.Wait()
	logging.Span("mcp_list_tools_parallel", parallelStart)
	for _, r := range results {
		if r.err != nil {
			return nil, nil, r.err
		}
		metas = append(metas, r.metas...)
		defs = append(defs, r.defs...)
	}

	return metas, defs, nil
}

// ensureClientLocked 确保给定配置对应的客户端可用，必要时按最新配置重建连接
func (m *Manager) ensureClientLocked(item domain.MCPConfig) (*managedClient, error) {
	existing, ok := m.clients[item.ID]
	if ok && existing.raw == item.Config {
		// 配置未变化时复用已有连接，减少重复握手与进程创建开销。
		return existing, nil
	}
	if ok {
		existing.clientMu.Lock()
		_ = existing.client.Close()
		existing.clientMu.Unlock()
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
		err = fmt.Errorf("unsupported transport: %s", cfg.Transport)
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

// Execute 根据 serverAlias 与 toolName 定位目标 MCP 服务并执行工具调用。
func (m *Manager) Execute(ctx context.Context, configs []domain.MCPConfig, serverAlias, toolName string, arguments map[string]any) (*CallResult, error) {
	var target domain.MCPConfig
	found := false
	var entry *managedClient

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
		return nil, fmt.Errorf("MCP service not found or disabled: %s", serverAlias)
	}
	m.mu.Lock()
	var err error
	entry, err = m.ensureClientLocked(target)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}

	entry.clientMu.Lock()
	res, callErr := entry.client.CallTool(ctx, toolName, arguments)
	entry.clientMu.Unlock()
	return res, callErr
}

// sanitizeToken 将任意标识规范化为安全 token，用于函数名与别名拼接。
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

// buildMCPFuncName 生成 server__tool 函数名；超长时截断两段并追加 SHA1 前缀以满足长度上限且降低冲突。
func buildMCPFuncName(serverAlias, toolName string) string {
	serverToken := sanitizeToken(serverAlias)
	toolToken := sanitizeToken(toolName)
	full := serverToken + "__" + toolToken
	if len(full) <= constants.MCPFuncNameMaxLen {
		return full
	}

	// 保留可读前缀，并追加稳定哈希，兼顾长度限制与冲突概率。
	sum := sha1.Sum([]byte(serverToken + "::" + toolToken))
	hash := hex.EncodeToString(sum[:])
	if len(hash) > constants.MCPFuncHashLen {
		hash = hash[:constants.MCPFuncHashLen]
	}

	// 预留 "__" 与 "_<hash>"。
	available := constants.MCPFuncNameMaxLen - len("__") - 1 - len(hash)
	if available < 2 {
		available = 2
	}
	serverLen := available / 2
	toolLen := available - serverLen

	shortServer := truncateToken(serverToken, serverLen)
	shortTool := truncateToken(toolToken, toolLen)
	name := shortServer + "__" + shortTool + "_" + hash
	if len(name) <= constants.MCPFuncNameMaxLen {
		return name
	}
	return name[:constants.MCPFuncNameMaxLen]
}

func truncateToken(input string, max int) string {
	if max <= 0 {
		return "x"
	}
	if input == "" {
		return "x"
	}
	if len(input) <= max {
		return input
	}
	return input[:max]
}
