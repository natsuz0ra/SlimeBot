package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"slimebot/internal/logging"

	"github.com/blevesearch/bleve/v2"
)

const (
	entrypointFileName = "MEMORY.md"
	bleveIndexDirName  = "index.bleve"
	maxEntrypointLines = 200
)

// FileMemoryStore 基于文件系统的记忆存储。
// 记忆以 Markdown + YAML frontmatter 存储在 baseDir 下，
// 同时维护 bleve 全文索引和 MEMORY.md 索引文件。
type FileMemoryStore struct {
	baseDir  string
	bleveIdx bleve.Index
	mu       sync.RWMutex
}

// NewFileMemoryStore 创建文件记忆存储。baseDir 通常为 ~/.slimebot/memory/。
func NewFileMemoryStore(baseDir string) (*FileMemoryStore, error) {
	absDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve memory dir: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	idx, err := openOrCreateBleveIndex(filepath.Join(absDir, bleveIndexDirName))
	if err != nil {
		return nil, fmt.Errorf("open bleve index: %w", err)
	}

	store := &FileMemoryStore{
		baseDir:  absDir,
		bleveIdx: idx,
	}

	return store, nil
}

// BaseDir 返回存储根目录。
func (s *FileMemoryStore) BaseDir() string {
	return s.baseDir
}

// Save 保存一条记忆：写入 .md 文件 + 更新 bleve 索引 + 更新 MEMORY.md。
// 如果同名 slug 已存在，保留原始创建时间并合并内容。
func (s *FileMemoryStore) Save(entry *MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 检查是否已存在同名记忆，存在则合并
	slug := entry.Slug()
	existingPath := filepath.Join(s.baseDir, slug+".md")
	if _, err := os.Stat(existingPath); err == nil {
		existing, parseErr := parseMemoryFile(existingPath)
		if parseErr == nil {
			entry.Created = existing.Created // 保留原始创建时间
			if entry.SessionID == "" {
				entry.SessionID = existing.SessionID // 保留原始会话 ID
			}
			if strings.TrimSpace(entry.Content) == "" {
				entry.Content = existing.Content
			}
			if strings.TrimSpace(entry.Description) == "" {
				entry.Description = existing.Description
			}
		}
	}

	if entry.Created.IsZero() {
		entry.Created = now
	}
	entry.Updated = now

	fileName := entry.FileName()
	entry.FilePath = filepath.Join(s.baseDir, fileName)

	// 写入 markdown 文件
	content := formatMemoryFile(entry)
	if err := os.WriteFile(entry.FilePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write memory file %s: %w", fileName, err)
	}

	// 更新 bleve 索引
	if err := indexBleveDocument(s.bleveIdx, entry); err != nil {
		logging.Warn("bleve_index_failed", "file", fileName, "error", err)
	}

	// 更新 MEMORY.md 索引
	if err := rebuildEntrypoint(s.baseDir, maxEntrypointLines); err != nil {
		logging.Warn("rebuild_memory_index_failed", "error", err)
	}

	logging.Info("memory_saved", "name", entry.Name, "type", entry.Type, "file", fileName)
	return nil
}

// Load 按名称加载一条记忆。
func (s *FileMemoryStore) Load(name string) (*MemoryEntry, error) {
	slug := Slugify(name)
	filePath := filepath.Join(s.baseDir, slug+".md")
	return parseMemoryFile(filePath)
}

// Delete 删除一条记忆。
func (s *FileMemoryStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	slug := Slugify(name)
	filePath := filepath.Join(s.baseDir, slug+".md")

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete memory file %s: %w", slug, err)
	}

	// 从 bleve 索引删除
	if err := s.bleveIdx.Delete(slug); err != nil {
		logging.Warn("bleve_delete_failed", "slug", slug, "error", err)
	}

	// 重建 MEMORY.md
	if err := rebuildEntrypoint(s.baseDir, maxEntrypointLines); err != nil {
		logging.Warn("rebuild_memory_index_failed", "error", err)
	}

	logging.Info("memory_deleted", "name", name, "slug", slug)
	return nil
}

// List 列出所有记忆条目的元信息（不含正文）。
func (s *FileMemoryStore) List() ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return scanMemoryDir(s.baseDir)
}

// Scan 扫描所有记忆条目（含正文）。
func (s *FileMemoryStore) Scan() ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return scanMemoryDirFull(s.baseDir)
}

// Search 使用 bleve 全文索引搜索记忆。
func (s *FileMemoryStore) Search(query string, topK int) ([]*MemoryEntry, error) {
	if topK <= 0 {
		topK = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	slugs, err := searchBleveIndex(s.bleveIdx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("search bleve: %w", err)
	}

	var results []*MemoryEntry
	for _, slug := range slugs {
		entry, parseErr := parseMemoryFile(filepath.Join(s.baseDir, slug+".md"))
		if parseErr != nil {
			logging.Warn("memory_parse_failed", "slug", slug, "error", parseErr)
			continue
		}
		results = append(results, entry)
	}

	return results, nil
}

// SearchBySession 使用 bleve 全文索引搜索指定会话的记忆。
func (s *FileMemoryStore) SearchBySession(sessionID, query string, topK int) ([]*MemoryEntry, error) {
	if topK <= 0 {
		topK = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	slugs, err := searchBleveIndexBySession(s.bleveIdx, sessionID, query, topK)
	if err != nil {
		return nil, fmt.Errorf("search bleve by session: %w", err)
	}

	var results []*MemoryEntry
	for _, slug := range slugs {
		entry, parseErr := parseMemoryFile(filepath.Join(s.baseDir, slug+".md"))
		if parseErr != nil {
			logging.Warn("memory_parse_failed", "slug", slug, "error", parseErr)
			continue
		}
		results = append(results, entry)
	}

	return results, nil
}

// RebuildIndex 重建 MEMORY.md 索引和 bleve 索引。
func (s *FileMemoryStore) RebuildIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 扫描所有文件
	entries, err := scanMemoryDirFull(s.baseDir)
	if err != nil {
		return fmt.Errorf("scan memory dir: %w", err)
	}

	// 重建 bleve 索引
	indexPath := filepath.Join(s.baseDir, bleveIndexDirName)
	if err := os.RemoveAll(indexPath); err != nil {
		return fmt.Errorf("remove old bleve index: %w", err)
	}

	idx, err := openOrCreateBleveIndex(indexPath)
	if err != nil {
		return fmt.Errorf("create bleve index: %w", err)
	}
	s.bleveIdx = idx

	batch := s.bleveIdx.NewBatch()
	for _, entry := range entries {
		doc := entry.ToBleveDocument()
		if err := batch.Index(entry.Slug(), doc); err != nil {
			logging.Warn("bleve_batch_index_failed", "slug", entry.Slug(), "error", err)
		}
	}
	if err := s.bleveIdx.Batch(batch); err != nil {
		return fmt.Errorf("bleve batch: %w", err)
	}

	// 重建 MEMORY.md
	if err := rebuildEntrypoint(s.baseDir, maxEntrypointLines); err != nil {
		return fmt.Errorf("rebuild entrypoint: %w", err)
	}

	logging.Info("memory_index_rebuilt", "entries", len(entries))
	return nil
}

// Close 关闭 bleve 索引。
func (s *FileMemoryStore) Close() error {
	if s.bleveIdx != nil {
		return s.bleveIdx.Close()
	}
	return nil
}

// ReadEntrypoint 读取 MEMORY.md 内容。
func (s *FileMemoryStore) ReadEntrypoint() string {
	data, err := os.ReadFile(filepath.Join(s.baseDir, entrypointFileName))
	if err != nil {
		return ""
	}
	return string(data)
}

// formatMemoryFile 格式化为 markdown + frontmatter。
func formatMemoryFile(entry *MemoryEntry) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", entry.Name))
	b.WriteString(fmt.Sprintf("description: %s\n", entry.Description))
	b.WriteString(fmt.Sprintf("type: %s\n", entry.Type))
	if entry.SessionID != "" {
		b.WriteString(fmt.Sprintf("session_id: %s\n", entry.SessionID))
	}
	b.WriteString(fmt.Sprintf("created: %s\n", entry.Created.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("updated: %s\n", entry.Updated.Format(time.RFC3339)))
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(entry.Content))
	b.WriteString("\n")
	return b.String()
}
