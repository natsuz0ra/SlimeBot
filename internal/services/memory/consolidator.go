package memory

import (
	"fmt"
	"slimebot/internal/logging"
	"strings"
)

// Consolidator 记忆整合器，定期合并碎片记忆并清理冗余。
// 参考 Claude Code 的 autoDream 服务。
type Consolidator struct {
	store *FileMemoryStore
}

// NewConsolidator 创建整合器。
func NewConsolidator(store *FileMemoryStore) *Consolidator {
	return &Consolidator{store: store}
}

// Run 执行一次整合：扫描所有记忆，合并同类型同主题的碎片。
// 返回 (合并数, 删除数, error)。
func (c *Consolidator) Run() (merged int, deleted int, err error) {
	entries, err := c.store.Scan()
	if err != nil {
		return 0, 0, fmt.Errorf("scan memories for consolidation: %w", err)
	}

	if len(entries) < 2 {
		return 0, 0, nil
	}

	// 按类型分组
	grouped := make(map[MemoryType][]*MemoryEntry)
	for _, e := range entries {
		grouped[e.Type] = append(grouped[e.Type], e)
	}

	var toDelete []string
	var toCreate []*MemoryEntry

	for _, group := range grouped {
		mergedSet := make(map[string]bool)
		for i := 0; i < len(group); i++ {
			if mergedSet[group[i].Slug()] {
				continue
			}
			for j := i + 1; j < len(group); j++ {
				if mergedSet[group[j].Slug()] {
					continue
				}
				if shouldMerge(group[i], group[j]) {
					merged := mergeEntries(group[i], group[j])
					toCreate = append(toCreate, merged)
					toDelete = append(toDelete, group[i].Slug(), group[j].Slug())
					mergedSet[group[i].Slug()] = true
					mergedSet[group[j].Slug()] = true
					break
				}
			}
		}
	}

	// 先删除旧条目
	for _, slug := range toDelete {
		if delErr := c.store.Delete(slug); delErr != nil {
			logging.Warn("consolidator_delete_failed", "slug", slug, "error", delErr)
		}
	}

	// 再创建合并后的新条目
	for _, entry := range toCreate {
		if saveErr := c.store.Save(entry); saveErr != nil {
			logging.Warn("consolidator_save_failed", "name", entry.Name, "error", saveErr)
		}
	}

	merged = len(toCreate)
	deleted = len(toDelete)
	logging.Info("consolidator_completed", "merged", merged, "deleted", deleted)
	return merged, deleted, nil
}

// shouldMerge 判断两条记忆是否应该合并。
// 条件：同类型 + name 相同或 description 高度重叠。
func shouldMerge(a, b *MemoryEntry) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Name == b.Name {
		return true
	}
	return isSimilarDescription(a.Description, b.Description)
}

// isSimilarDescription 判断两个 description 是否高度相似。
// 简单实现：一方包含另一方且重叠比例超过 80%。
func isSimilarDescription(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	if strings.Contains(longer, shorter) && float64(len(shorter))/float64(len(longer)) > 0.8 {
		return true
	}
	return false
}

// mergeEntries 合并两条记忆为一条新记忆。
func mergeEntries(a, b *MemoryEntry) *MemoryEntry {
	var content strings.Builder
	content.WriteString(a.Content)
	if strings.TrimSpace(b.Content) != "" {
		content.WriteString("\n\n---\n\n")
		content.WriteString(b.Content)
	}

	created := a.Created
	if b.Created.Before(created) {
		created = b.Created
	}

	return &MemoryEntry{
		Name:        pickBetterName(a.Name, b.Name),
		Description: pickLonger(a.Description, b.Description),
		Type:        a.Type,
		Content:     content.String(),
		Created:     created,
	}
}

// pickBetterName 选择更有意义的名称。
func pickBetterName(a, b string) string {
	if len(a) >= len(b) {
		return a
	}
	return b
}

// pickLonger 返回较长的字符串。
func pickLonger(a, b string) string {
	if len(a) >= len(b) {
		return a
	}
	return b
}
