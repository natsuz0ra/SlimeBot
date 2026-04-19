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

// FileMemoryStore is file-backed memory storage.
// Each memory is Markdown + YAML frontmatter under baseDir,
// with a bleve full-text index and a MEMORY.md manifest.
type FileMemoryStore struct {
	baseDir  string
	bleveIdx bleve.Index
	mu       sync.RWMutex
}

// NewFileMemoryStore creates the store. baseDir is usually ~/.slimebot/memory/.
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

// BaseDir returns the storage root directory.
func (s *FileMemoryStore) BaseDir() string {
	return s.baseDir
}

// Save writes a memory: .md file + bleve index + MEMORY.md manifest.
// If the slug already exists, keeps original Created time and merges content.
func (s *FileMemoryStore) Save(entry *MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// If a memory with the same slug exists, merge with it.
	slug := entry.Slug()
	existingPath := filepath.Join(s.baseDir, slug+".md")
	if _, err := os.Stat(existingPath); err == nil {
		existing, parseErr := parseMemoryFile(existingPath)
		if parseErr == nil {
			entry.Created = existing.Created // preserve original creation time
			if entry.SessionID == "" {
				entry.SessionID = existing.SessionID // preserve original session ID
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

	// Write markdown file.
	content := formatMemoryFile(entry)
	if err := os.WriteFile(entry.FilePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write memory file %s: %w", fileName, err)
	}

	// Update bleve index.
	if err := indexBleveDocument(s.bleveIdx, entry); err != nil {
		logging.Warn("bleve_index_failed", "file", fileName, "error", err)
	}

	// Refresh MEMORY.md manifest.
	if err := rebuildEntrypoint(s.baseDir, maxEntrypointLines); err != nil {
		logging.Warn("rebuild_memory_index_failed", "error", err)
	}

	logging.Info("memory_saved", "name", entry.Name, "type", entry.Type, "file", fileName)
	return nil
}

// Load loads one memory by display name.
func (s *FileMemoryStore) Load(name string) (*MemoryEntry, error) {
	slug := Slugify(name)
	filePath := filepath.Join(s.baseDir, slug+".md")
	return parseMemoryFile(filePath)
}

// Delete removes one memory.
func (s *FileMemoryStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	slug := Slugify(name)
	filePath := filepath.Join(s.baseDir, slug+".md")

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete memory file %s: %w", slug, err)
	}

	// Remove from bleve index.
	if err := s.bleveIdx.Delete(slug); err != nil {
		logging.Warn("bleve_delete_failed", "slug", slug, "error", err)
	}

	// Rebuild MEMORY.md
	if err := rebuildEntrypoint(s.baseDir, maxEntrypointLines); err != nil {
		logging.Warn("rebuild_memory_index_failed", "error", err)
	}

	logging.Info("memory_deleted", "name", name, "slug", slug)
	return nil
}

// List returns metadata for all memories (no body text).
func (s *FileMemoryStore) List() ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return scanMemoryDir(s.baseDir)
}

// Scan returns all memories including body content.
func (s *FileMemoryStore) Scan() ([]*MemoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return scanMemoryDirFull(s.baseDir)
}

// Search runs a bleve full-text search over memories.
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

// SearchBySession searches memories scoped to one session via bleve.
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

// RebuildIndex rebuilds MEMORY.md and the bleve index.
func (s *FileMemoryStore) RebuildIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Scan all files.
	entries, err := scanMemoryDirFull(s.baseDir)
	if err != nil {
		return fmt.Errorf("scan memory dir: %w", err)
	}

	// Rebuild bleve index directory.
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

	// Rebuild MEMORY.md
	if err := rebuildEntrypoint(s.baseDir, maxEntrypointLines); err != nil {
		return fmt.Errorf("rebuild entrypoint: %w", err)
	}

	logging.Info("memory_index_rebuilt", "entries", len(entries))
	return nil
}

// Close closes the bleve index.
func (s *FileMemoryStore) Close() error {
	if s.bleveIdx != nil {
		return s.bleveIdx.Close()
	}
	return nil
}

// ReadEntrypoint reads MEMORY.md content.
func (s *FileMemoryStore) ReadEntrypoint() string {
	data, err := os.ReadFile(filepath.Join(s.baseDir, entrypointFileName))
	if err != nil {
		return ""
	}
	return string(data)
}

// formatMemoryFile renders markdown with YAML frontmatter.
func formatMemoryFile(entry *MemoryEntry) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", entry.Name))
	b.WriteString(fmt.Sprintf("description: %s\n", entry.Description))
	b.WriteString(fmt.Sprintf("type: %s\n", entry.Type))
	if entry.SessionID != "" {
		b.WriteString(fmt.Sprintf("session_id: %s\n", entry.SessionID))
	}
	b.WriteString(fmt.Sprintf("created: %s\n", entry.Created.Format(time.RFC3339Nano)))
	b.WriteString(fmt.Sprintf("updated: %s\n", entry.Updated.Format(time.RFC3339Nano)))
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(entry.Content))
	b.WriteString("\n")
	return b.String()
}
