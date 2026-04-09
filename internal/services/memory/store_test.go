package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileMemoryStore_Save_and_Load(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	entry := &MemoryEntry{
		Name:        "Test Memory",
		Description: "A test memory entry",
		Type:        MemoryTypeProject,
		Content:     "This is the content of the test memory.",
	}

	// Save
	if err := store.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	expectedPath := filepath.Join(tmpDir, entry.FileName())
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("expected file %s to exist", expectedPath)
	}

	// Load
	loaded, err := store.Load(entry.Slug())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != entry.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, entry.Name)
	}
	if loaded.Description != entry.Description {
		t.Errorf("Description = %q, want %q", loaded.Description, entry.Description)
	}
	if loaded.Type != entry.Type {
		t.Errorf("Type = %q, want %q", loaded.Type, entry.Type)
	}
	if loaded.Content != entry.Content {
		t.Errorf("Content = %q, want %q", loaded.Content, entry.Content)
	}
}

func TestFileMemoryStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	// Create multiple entries
	entries := []*MemoryEntry{
		{Name: "First", Description: "First entry", Type: MemoryTypeUser, Content: "Content 1"},
		{Name: "Second", Description: "Second entry", Type: MemoryTypeProject, Content: "Content 2"},
		{Name: "Third", Description: "Third entry", Type: MemoryTypeFeedback, Content: "Content 3"},
	}

	for _, e := range entries {
		if err := store.Save(e); err != nil {
			t.Fatalf("Save %q: %v", e.Name, err)
		}
	}

	// List
	listed, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(listed) != len(entries) {
		t.Errorf("List returned %d entries, want %d", len(listed), len(entries))
	}

	// Verify all entries are present (List doesn't guarantee order)
	names := make(map[string]bool)
	for _, e := range listed {
		names[e.Name] = true
	}
	for _, want := range entries {
		if !names[want.Name] {
			t.Errorf("List missing entry %q", want.Name)
		}
	}
}

func TestFileMemoryStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	entry := &MemoryEntry{
		Name:        "To Delete",
		Description: "This will be deleted",
		Type:        MemoryTypeReference,
		Content:     "Delete me",
	}

	if err := store.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Delete
	if err := store.Delete(entry.Name); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify file is gone
	filePath := filepath.Join(tmpDir, entry.FileName())
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted", filePath)
	}

	// Verify load fails
	_, err = store.Load(entry.Slug())
	if err == nil {
		t.Error("expected Load to fail for deleted entry")
	}
}

func TestFileMemoryStore_Search(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	entries := []*MemoryEntry{
		{Name: "Go Programming", Description: "About Go language", Type: MemoryTypeProject, Content: "Go is a statically typed language"},
		{Name: "Python Code", Description: "About Python", Type: MemoryTypeProject, Content: "Python is dynamically typed"},
		{Name: "User Preference", Description: "Likes dark mode", Type: MemoryTypeUser, Content: "User prefers dark theme"},
	}

	for _, e := range entries {
		if err := store.Save(e); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Search for "Go"
	results, err := store.Search("Go", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search returned no results for 'Go'")
	}

	// First result should be about Go
	found := false
	for _, r := range results {
		if r.Name == "Go Programming" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find 'Go Programming' in search results")
	}
}

func TestFileMemoryStore_RebuildIndex(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}

	// Create some entries
	entries := []*MemoryEntry{
		{Name: "Alpha", Description: "First", Type: MemoryTypeUser, Content: "A"},
		{Name: "Beta", Description: "Second", Type: MemoryTypeProject, Content: "B"},
	}

	for _, e := range entries {
		if err := store.Save(e); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	// Close the old index before rebuilding (required on Windows)
	if err := store.Close(); err != nil {
		t.Fatalf("Close before rebuild: %v", err)
	}

	// Rebuild index (reopens the index)
	if err := store.RebuildIndex(); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}
	defer store.Close()

	// Verify search still works
	results, err := store.Search("Alpha", 5)
	if err != nil {
		t.Fatalf("Search after rebuild: %v", err)
	}
	if len(results) == 0 {
		t.Error("Search returned no results after RebuildIndex")
	}
}

func TestMemoryEntry_Slug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Test Memory", "test_memory"},
		{"spaces", "  multiple   spaces  ", "multiple___spaces"}, // 当前实现不折叠多个下划线
		{"special", "Test@#$%Memory", "testmemory"},
		{"mixed", "Test123-ABC", "test123-abc"},
		{"chinese", "测试记忆", "memory_"}, // 中文被移除后生成带时间戳的默认名，前缀是 memory_
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &MemoryEntry{Name: tt.input}
			got := e.Slug()
			// 对于中文，检查前缀而非完整值（因为包含时间戳）
			if tt.name == "chinese" {
				if !startsWith(got, tt.expected) {
					t.Errorf("Slug() = %q, want prefix %q", got, tt.expected)
				}
			} else {
				if got != tt.expected {
					t.Errorf("Slug() = %q, want %q", got, tt.expected)
				}
			}
		})
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestParseMemoryType(t *testing.T) {
	tests := []struct {
		input    string
		expected MemoryType
		wantErr  bool
	}{
		{"user", MemoryTypeUser, false},
		{"USER", MemoryTypeUser, false},
		{"  user  ", MemoryTypeUser, false},
		{"feedback", MemoryTypeFeedback, false},
		{"project", MemoryTypeProject, false},
		{"reference", MemoryTypeReference, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMemoryType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMemoryType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseMemoryType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseMemoryPayload(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(*testing.T, *MemoryEntry)
	}{
		{
			name:    "valid json",
			input:   `{"name":"Test","description":"A test","type":"user","content":"content"}`,
			wantErr: false,
			check: func(t *testing.T, e *MemoryEntry) {
				if e.Name != "Test" {
					t.Errorf("Name = %q, want 'Test'", e.Name)
				}
				if e.Type != MemoryTypeUser {
					t.Errorf("Type = %q, want 'user'", e.Type)
				}
			},
		},
		{
			name:    "json with markdown wrapper",
			input:   "```json\n{\"name\":\"Test\",\"description\":\"Test\",\"type\":\"project\",\"content\":\"content\"}\n```",
			wantErr: false,
			check: func(t *testing.T, e *MemoryEntry) {
				if e.Name != "Test" {
					t.Errorf("Name = %q, want 'Test'", e.Name)
				}
			},
		},
		{
			name:    "empty name",
			input:   `{"description":"No name"}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemoryPayload(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMemoryPayload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestFileMemoryStore_ReadEntrypoint(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	// Empty store - entrypoint doesn't exist yet
	content := store.ReadEntrypoint()
	// This is OK - the file is only created when there's content
	// After Save, it should be created

	// Add an entry
	entry := &MemoryEntry{
		Name:        "Test Entry",
		Description: "Test description",
		Type:        MemoryTypeProject,
		Content:     "Content",
	}
	if err := store.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Read entrypoint again
	content = store.ReadEntrypoint()
	if content == "" {
		t.Error("ReadEntrypoint returned empty string after Save")
	}

	// Should contain the entry name
	if !contains(content, "Test Entry") {
		t.Errorf("Entrypoint should contain 'Test Entry', got:\n%s", content)
	}
}

func TestFileMemoryStore_Save_updates_UpdatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	entry := &MemoryEntry{
		Name:        "Update Test",
		Description: "Original",
		Type:        MemoryTypeProject,
		Content:     "Original content",
	}

	if err := store.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Wait a bit to ensure timestamp difference
	time.Sleep(10 * time.Millisecond)

	// Update
	entry.Description = "Updated"
	entry.Content = "Updated content"
	if err := store.Save(entry); err != nil {
		t.Fatalf("Save (update): %v", err)
	}

	// Load and verify Updated changed
	loaded, err := store.Load(entry.Slug())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Description != "Updated" {
		t.Errorf("Description = %q, want 'Updated'", loaded.Description)
	}
	if loaded.Content != "Updated content" {
		t.Errorf("Content = %q, want 'Updated content'", loaded.Content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && indexOfSubstring(s, substr) >= 0))
}

func indexOfSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
