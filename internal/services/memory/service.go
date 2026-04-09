package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"slimebot/internal/logging"
	"strings"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// MemoryService 记忆服务，封装 FileMemoryStore 对外提供统一接口。
// 保持与 chat/agent 服务的兼容性。
type MemoryService struct {
	store *FileMemoryStore
}

// MemorySearchHit 记忆搜索结果条目。
type MemorySearchHit struct {
	Kind      string
	ID        string
	Title     string
	Summary   string
	Score     float64
	CreatedAt time.Time
}

// MemoryQueryResult 记忆搜索结果。
type MemoryQueryResult struct {
	Query  string
	Hits   []MemorySearchHit
	Output string
}

// NewMemoryService 创建记忆服务。baseDir 通常为 ~/.slimebot/memory/。
func NewMemoryService(baseDir string) (*MemoryService, error) {
	store, err := NewFileMemoryStore(baseDir)
	if err != nil {
		return nil, fmt.Errorf("create file memory store: %w", err)
	}
	return &MemoryService{store: store}, nil
}

// Shutdown 关闭服务。
func (m *MemoryService) Shutdown(ctx context.Context) error {
	if m == nil || m.store == nil {
		return nil
	}
	return m.store.Close()
}

// BuildMemoryContext 构建记忆上下文，注入到聊天提示中。
func (m *MemoryService) BuildMemoryContext(ctx context.Context, sessionID string, history []domain.Message) string {
	if m == nil || m.store == nil {
		return ""
	}
	return m.buildMemoryContext(ctx, sessionID, history)
}

// BuildSessionMemoryContextForPrompt 别名，兼容旧接口。
func (m *MemoryService) BuildSessionMemoryContextForPrompt(ctx context.Context, sessionID string, history []domain.Message) string {
	return m.BuildMemoryContext(ctx, sessionID, history)
}

// BuildRecentHistory 获取最近历史消息。
func (m *MemoryService) BuildRecentHistory(sessionID string, limit int) ([]domain.Message, error) {
	// 新方案不再维护历史，返回空
	return nil, nil
}

// QueryForAgent 为 Agent 工具调用搜索记忆。
func (m *MemoryService) QueryForAgent(ctx context.Context, sessionID string, query string, topK int) (MemoryQueryResult, error) {
	result := MemoryQueryResult{Query: strings.TrimSpace(query)}
	if result.Query == "" {
		return result, fmt.Errorf("memory_query query cannot be empty")
	}
	if topK <= 0 {
		topK = constants.MemoryToolDefaultTopK
	}

	entries, err := m.store.Search(result.Query, topK)
	if err != nil {
		return result, fmt.Errorf("search memory: %w", err)
	}

	for _, entry := range entries {
		result.Hits = append(result.Hits, MemorySearchHit{
			Kind:      string(entry.Type),
			ID:        entry.Slug(),
			Title:     entry.Name,
			Summary:   truncateContent(entry.Content, 200),
			Score:     1.0, // bleve 内部排序已处理
			CreatedAt: entry.Created,
		})
	}

	result.Output = buildMemoryQueryOutput(result.Query, nil, result.Hits)
	return result, nil
}

// EnqueueTurnMemory 处理模型输出的记忆 payload。
// 新方案：解析 JSON payload，检查去重后写入文件记忆。
func (m *MemoryService) EnqueueTurnMemory(sessionID, assistantMessageID, rawMemoryPayload string) {
	if m == nil || m.store == nil {
		return
	}
	payload := strings.TrimSpace(rawMemoryPayload)
	if payload == "" {
		return
	}
	logging.Info("memory_process_start", "session", sessionID, "payload_len", len(payload))

	// 尝试解析为 MemoryEntry JSON
	entry, err := parseMemoryPayload(payload)
	if err != nil {
		logging.Warn("memory_payload_parse_failed", "error", err)
		return
	}

	// 设置会话 ID
	entry.SessionID = sessionID

	// 去重检查：搜索是否有高度相似的记忆
	duplicates, _ := m.store.Search(entry.Name+" "+entry.Description, 3)
	for _, dup := range duplicates {
		if dup.Slug() == entry.Slug() {
			// 同名记忆，走更新逻辑（Save 内部处理合并）
			break
		}
		// 完全重复的 name 或 description，跳过
		if dup.Name == entry.Name || dup.Description == entry.Description {
			logging.Info("memory_duplicate_skipped", "name", entry.Name, "existing", dup.Name)
			return
		}
	}

	if err := m.store.Save(entry); err != nil {
		logging.Warn("memory_save_failed", "name", entry.Name, "error", err)
	}
}

// ReadEntrypoint 读取 MEMORY.md 内容。
func (m *MemoryService) ReadEntrypoint() string {
	if m == nil || m.store == nil {
		return ""
	}
	return m.store.ReadEntrypoint()
}

// Store 返回底层 FileMemoryStore（供测试或高级操作）。
func (m *MemoryService) Store() *FileMemoryStore {
	return m.store
}

// Consolidate 执行一次记忆整合，合并碎片记忆并清理冗余。
func (m *MemoryService) Consolidate() (merged int, deleted int, err error) {
	if m == nil || m.store == nil {
		return 0, 0, nil
	}
	return NewConsolidator(m.store).Run()
}

// buildMemoryContext 从 MEMORY.md 索引 + 相关记忆构建上下文。
// 使用对话历史作为检索 query，通过 bleve 搜索最相关的记忆注入，
// 而非全量注入最近记忆。参考 Claude Code 的 findRelevantMemories。
func (m *MemoryService) buildMemoryContext(ctx context.Context, sessionID string, history []domain.Message) string {
	// 读取 MEMORY.md 索引
	entrypoint := m.store.ReadEntrypoint()
	if strings.TrimSpace(entrypoint) == "" {
		return ""
	}

	// 第一层：始终注入 manifest（MEMORY.md 索引）
	var b strings.Builder
	b.WriteString("<memory_index>\n")
	b.WriteString(entrypoint)
	b.WriteString("\n</memory_index>\n")

	// 第二层：用对话历史检索当前会话相关记忆的完整内容
	query := extractSearchQuery(history, 3)
	if query == "" {
		return b.String()
	}

	entries, err := m.store.SearchBySession(sessionID, query, constants.MemoryContextTopK)
	if err != nil || len(entries) == 0 {
		// 会话记忆检索失败或无结果，不回退到全局记忆（保持会话隔离）
		return b.String()
	}

	if len(entries) == 0 {
		return b.String()
	}

	b.WriteString("<relevant_memories>\n")
	for _, entry := range entries {
		freshness := freshnessLabel(entry.Updated)
		if freshness != "" {
			b.WriteString(fmt.Sprintf("## %s (%s) %s\n", entry.Name, entry.Type, freshness))
		} else {
			b.WriteString(fmt.Sprintf("## %s (%s)\n", entry.Name, entry.Type))
		}
		// 注入完整内容而非仅 description
		if strings.TrimSpace(entry.Content) != "" {
			b.WriteString(entry.Content)
		} else {
			b.WriteString(entry.Description)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("</relevant_memories>")

	return b.String()
}

// extractSearchQuery 从最近几轮对话中提取搜索文本。
// 取最近 lastN 轮中用户消息的文本拼接作为检索 query。
func extractSearchQuery(history []domain.Message, lastN int) string {
	start := len(history) - lastN*2
	if start < 0 {
		start = 0
	}
	var parts []string
	for i := start; i < len(history); i++ {
		if history[i].Role == "user" {
			content := strings.TrimSpace(history[i].Content)
			if content != "" {
				parts = append(parts, content)
			}
		}
	}
	query := strings.Join(parts, " ")
	if len(query) > 200 {
		query = query[:200]
	}
	return query
}

// parseMemoryPayload 解析模型输出的 JSON payload 为 MemoryEntry。
func parseMemoryPayload(raw string) (*MemoryEntry, error) {
	// 去掉可能的 markdown 代码块包装
	cleaned := strings.TrimSpace(raw)
	if strings.HasPrefix(cleaned, "```") {
		// 去掉 ```json 或 ``` 等标记
		firstNewline := strings.Index(cleaned, "\n")
		if firstNewline > 0 {
			cleaned = cleaned[firstNewline+1:]
		} else {
			cleaned = cleaned[3:]
		}
		// 去掉结尾的 ```
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	// 尝试解析为标准 JSON
	type payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Content     string `json:"content"`
	}

	var p payload
	if err := json.Unmarshal([]byte(cleaned), &p); err != nil {
		return nil, fmt.Errorf("unmarshal memory payload: %w", err)
	}

	if p.Name == "" {
		return nil, fmt.Errorf("empty name in memory payload")
	}

	memType, err := ParseMemoryType(p.Type)
	if err != nil {
		memType = MemoryTypeProject // 默认类型
	}

	return &MemoryEntry{
		Name:        p.Name,
		Description: p.Description,
		Type:        memType,
		Content:     p.Content,
	}, nil
}

// freshnessLabel 根据记忆更新时间返回新鲜度标注，参考 Claude Code 的 memoryAgeDays。
func freshnessLabel(updated time.Time) string {
	days := int(time.Since(updated).Hours() / 24)
	switch {
	case days <= 1:
		return ""
	case days <= 7:
		return fmt.Sprintf("[%d天前]", days)
	case days <= 30:
		return fmt.Sprintf("[%d天前，可能过时]", days)
	default:
		return fmt.Sprintf("[%d天前，需要验证]", days)
	}
}

// truncateContent 截断内容到指定字符数。
func truncateContent(s string, maxRunes int) string {
	if len([]rune(s)) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}

// buildMemoryQueryOutput 格式化搜索结果为 XML 输出。
func buildMemoryQueryOutput(query string, keywords []string, hits []MemorySearchHit) string {
	var b strings.Builder
	b.WriteString("<memory_query_result>\n")
	b.WriteString("query: ")
	b.WriteString(strings.TrimSpace(query))
	b.WriteString("\nkeywords: ")
	if len(keywords) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(keywords, ", "))
	}
	b.WriteString(fmt.Sprintf("\nhit_count: %d\n", len(hits)))
	if len(hits) == 0 {
		b.WriteString("No related memories found.\n</memory_query_result>")
		return b.String()
	}
	for idx, item := range hits {
		b.WriteString(fmt.Sprintf("- [%d] %s | %s | %.2f | %s\n", idx+1, item.Kind, item.Title, item.Score, item.CreatedAt.Format(time.RFC3339)))
		b.WriteString("  ")
		b.WriteString(strings.TrimSpace(item.Summary))
		b.WriteString("\n")
	}
	b.WriteString("</memory_query_result>")
	return strings.TrimSpace(b.String())
}
