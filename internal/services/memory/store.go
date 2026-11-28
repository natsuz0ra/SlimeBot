package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) *Store {
	return &Store{dir: strings.TrimSpace(dir)}
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) Load(ctx context.Context, target Target, limit int) (TargetState, error) {
	if err := ctx.Err(); err != nil {
		return TargetState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadEntriesLocked(target)
	if err != nil {
		return TargetState{}, err
	}
	return buildTargetState(target, entries, limit, true), nil
}

func (s *Store) Add(ctx context.Context, target Target, content string, limit int) (TargetState, error) {
	if err := ctx.Err(); err != nil {
		return TargetState{}, err
	}
	content, err := normalizeEntry(content)
	if err != nil {
		return TargetState{}, err
	}
	if err := scanEntrySafety(content); err != nil {
		return TargetState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadEntriesLocked(target)
	if err != nil {
		return TargetState{}, err
	}
	for _, entry := range entries {
		if entry == content {
			return TargetState{}, ErrDuplicateEntry
		}
	}
	next := append(append([]string{}, entries...), content)
	if usageChars(next) > limit {
		return TargetState{}, ErrMemoryLimitExceeded
	}
	if err := s.saveEntriesLocked(target, next); err != nil {
		return TargetState{}, err
	}
	return buildTargetState(target, next, limit, true), nil
}

func (s *Store) Replace(ctx context.Context, target Target, oldText string, content string, limit int) (TargetState, error) {
	if err := ctx.Err(); err != nil {
		return TargetState{}, err
	}
	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return TargetState{}, ErrEntryNotFound
	}
	content, err := normalizeEntry(content)
	if err != nil {
		return TargetState{}, err
	}
	if err := scanEntrySafety(content); err != nil {
		return TargetState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadEntriesLocked(target)
	if err != nil {
		return TargetState{}, err
	}
	index, err := findUniqueEntry(entries, oldText)
	if err != nil {
		return TargetState{}, err
	}
	next := append([]string{}, entries...)
	next[index] = content
	if usageChars(next) > limit {
		return TargetState{}, ErrMemoryLimitExceeded
	}
	if err := s.saveEntriesLocked(target, next); err != nil {
		return TargetState{}, err
	}
	return buildTargetState(target, next, limit, true), nil
}

func (s *Store) Remove(ctx context.Context, target Target, oldText string) (TargetState, error) {
	if err := ctx.Err(); err != nil {
		return TargetState{}, err
	}
	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return TargetState{}, ErrEntryNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadEntriesLocked(target)
	if err != nil {
		return TargetState{}, err
	}
	index, err := findUniqueEntry(entries, oldText)
	if err != nil {
		return TargetState{}, err
	}
	next := append([]string{}, entries[:index]...)
	next = append(next, entries[index+1:]...)
	if err := s.saveEntriesLocked(target, next); err != nil {
		return TargetState{}, err
	}
	return buildTargetState(target, next, 0, true), nil
}

func (s *Store) RemoveIndex(ctx context.Context, target Target, index int) (TargetState, error) {
	if err := ctx.Err(); err != nil {
		return TargetState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadEntriesLocked(target)
	if err != nil {
		return TargetState{}, err
	}
	if index < 0 || index >= len(entries) {
		return TargetState{}, ErrEntryNotFound
	}
	next := append([]string{}, entries[:index]...)
	next = append(next, entries[index+1:]...)
	if err := s.saveEntriesLocked(target, next); err != nil {
		return TargetState{}, err
	}
	return buildTargetState(target, next, 0, true), nil
}

func (s *Store) Clear(ctx context.Context, target Target) (TargetState, error) {
	if err := ctx.Err(); err != nil {
		return TargetState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveEntriesLocked(target, nil); err != nil {
		return TargetState{}, err
	}
	return buildTargetState(target, nil, 0, true), nil
}

func (s *Store) loadEntriesLocked(target Target) ([]string, error) {
	path, err := s.pathForTarget(target)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, entrySeparator)
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		if entry := strings.TrimSpace(part); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (s *Store) saveEntriesLocked(target Target, entries []string) error {
	path, err := s.pathForTarget(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content := strings.Join(entries, entrySeparator)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *Store) pathForTarget(target Target) (string, error) {
	filename, err := targetFilename(target)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s.dir) == "" {
		return "", fmt.Errorf("memory directory is empty")
	}
	return filepath.Join(s.dir, filename), nil
}

func targetFilename(target Target) (string, error) {
	switch target {
	case TargetMemory:
		return "MEMORY.md", nil
	case TargetUser:
		return "USER.md", nil
	default:
		return "", ErrInvalidTarget
	}
}

func normalizeEntry(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", ErrEmptyEntry
	}
	if !utf8.ValidString(content) {
		return "", ErrUnsafeMemory
	}
	return content, nil
}

func findUniqueEntry(entries []string, oldText string) (int, error) {
	matches := make([]int, 0, 1)
	for i, entry := range entries {
		if strings.Contains(entry, oldText) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return -1, ErrEntryNotFound
	case 1:
		return matches[0], nil
	default:
		return -1, ErrAmbiguousMatch
	}
}

func buildTargetState(target Target, entries []string, limit int, enabled bool) TargetState {
	copied := append([]string{}, entries...)
	return TargetState{
		Target:     target,
		Entries:    copied,
		UsageChars: usageChars(copied),
		CharLimit:  limit,
		EntryCount: len(copied),
		Enabled:    enabled,
	}
}

func usageChars(entries []string) int {
	if len(entries) == 0 {
		return 0
	}
	return len([]rune(strings.Join(entries, entrySeparator)))
}
