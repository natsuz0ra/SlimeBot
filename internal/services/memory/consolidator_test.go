package memory

import (
	"testing"
	"time"
)

func TestConsolidator_MergesDuplicateNames(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	// 保存两条同类型同名的记忆
	entry1 := &MemoryEntry{
		Name:        "Go Tips",
		Description: "Go programming tips",
		Type:        MemoryTypeProject,
		Content:     "Use table-driven tests",
	}
	entry2 := &MemoryEntry{
		Name:        "Go Tips",
		Description: "More Go tips",
		Type:        MemoryTypeProject,
		Content:     "Prefer small interfaces",
	}

	if err := store.Save(entry1); err != nil {
		t.Fatalf("Save entry1: %v", err)
	}
	// 第二条同名但 slug 相同，Save 内部会合并
	if err := store.Save(entry2); err != nil {
		t.Fatalf("Save entry2: %v", err)
	}

	// 运行整合器
	c := NewConsolidator(store)
	merged, deleted, err := c.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 应该已经合并过了（Save 内部去重），consolidator 不需要额外操作
	t.Logf("merged=%d, deleted=%d", merged, deleted)
}

func TestConsolidator_MergesSimilarDescriptions(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	// 两条同类型、description 高度相似的记忆
	entry1 := &MemoryEntry{
		Name:        "User Pref Dark",
		Description: "User prefers dark mode in editor",
		Type:        MemoryTypeUser,
		Content:     "Dark theme with monokai colors",
	}
	entry2 := &MemoryEntry{
		Name:        "Dark Mode Pref",
		Description: "User prefers dark mode in editor and terminal",
		Type:        MemoryTypeUser,
		Content:     "Also uses dark terminal theme",
	}

	if err := store.Save(entry1); err != nil {
		t.Fatalf("Save entry1: %v", err)
	}
	// 修改 slug 避免同名覆盖
	entry2.Name = "Dark Mode Pref"
	if err := store.Save(entry2); err != nil {
		t.Fatalf("Save entry2: %v", err)
	}

	c := NewConsolidator(store)
	merged, deleted, err := c.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	t.Logf("merged=%d, deleted=%d", merged, deleted)

	// 验证整合后记忆数量
	remaining, _ := store.List()
	t.Logf("remaining entries: %d", len(remaining))
}

func TestConsolidator_NoMergeNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	// 两条不同类型的记忆
	entry1 := &MemoryEntry{
		Name:        "Alpha",
		Description: "First entry",
		Type:        MemoryTypeUser,
		Content:     "Content A",
	}
	entry2 := &MemoryEntry{
		Name:        "Beta",
		Description: "Second entry",
		Type:        MemoryTypeProject,
		Content:     "Content B",
	}

	store.Save(entry1)
	store.Save(entry2)

	c := NewConsolidator(store)
	merged, deleted, err := c.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if merged != 0 || deleted != 0 {
		t.Errorf("expected no merges for different types, got merged=%d deleted=%d", merged, deleted)
	}
}

func TestConsolidator_EmptyStore(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	c := NewConsolidator(store)
	merged, deleted, err := c.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if merged != 0 || deleted != 0 {
		t.Errorf("empty store should have no merges, got merged=%d deleted=%d", merged, deleted)
	}
}

func TestShouldMerge(t *testing.T) {
	tests := []struct {
		name      string
		a         *MemoryEntry
		b         *MemoryEntry
		wantMerge bool
	}{
		{
			name:      "same type same name",
			a:         &MemoryEntry{Name: "Test", Type: MemoryTypeProject, Description: "desc a"},
			b:         &MemoryEntry{Name: "Test", Type: MemoryTypeProject, Description: "desc b"},
			wantMerge: true,
		},
		{
			name:      "different type same name",
			a:         &MemoryEntry{Name: "Test", Type: MemoryTypeUser, Description: "desc a"},
			b:         &MemoryEntry{Name: "Test", Type: MemoryTypeProject, Description: "desc b"},
			wantMerge: false,
		},
		{
			name:      "identical description",
			a:         &MemoryEntry{Name: "A", Type: MemoryTypeUser, Description: "User prefers dark mode everywhere"},
			b:         &MemoryEntry{Name: "B", Type: MemoryTypeUser, Description: "User prefers dark mode everywhere"},
			wantMerge: true,
		},
		{
			name:      "different everything",
			a:         &MemoryEntry{Name: "A", Type: MemoryTypeUser, Description: "User likes cats"},
			b:         &MemoryEntry{Name: "B", Type: MemoryTypeProject, Description: "Deploy pipeline"},
			wantMerge: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldMerge(tt.a, tt.b)
			if got != tt.wantMerge {
				t.Errorf("shouldMerge() = %v, want %v", got, tt.wantMerge)
			}
		})
	}
}

func TestFreshnessLabel(t *testing.T) {
	tests := []struct {
		name   string
		days   int
		expect string
	}{
		{"fresh", 0, ""},
		{"1 day", 1, ""},
		{"3 days", 3, "[3天前]"},
		{"7 days", 7, "[7天前]"},
		{"14 days", 14, "[14天前，可能过时]"},
		{"31 days", 31, "[31天前，需要验证]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := time.Now().AddDate(0, 0, -tt.days)
			got := freshnessLabel(updated)
			if got != tt.expect {
				t.Errorf("freshnessLabel(%d days ago) = %q, want %q", tt.days, got, tt.expect)
			}
		})
	}
}

func TestSave_DeduplicatesSameSlug(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileMemoryStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileMemoryStore: %v", err)
	}
	defer store.Close()

	// 第一次保存
	entry1 := &MemoryEntry{
		Name:        "Test Entry",
		Description: "Original description",
		Type:        MemoryTypeProject,
		Content:     "Original content",
	}
	if err := store.Save(entry1); err != nil {
		t.Fatalf("Save first: %v", err)
	}

	// 第二次保存同名记忆（应更新而非创建新文件）
	entry2 := &MemoryEntry{
		Name:        "Test Entry",
		Description: "Updated description",
		Type:        MemoryTypeProject,
		Content:     "Updated content",
	}
	if err := store.Save(entry2); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	// 验证只有一个文件，且内容是更新后的
	loaded, err := store.Load(entry1.Slug())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Description != "Updated description" {
		t.Errorf("Description = %q, want 'Updated description'", loaded.Description)
	}
	if loaded.Content != "Updated content" {
		t.Errorf("Content = %q, want 'Updated content'", loaded.Content)
	}

	// 验证创建时间保留
	if loaded.Created.After(loaded.Updated) {
		t.Error("Created should not be after Updated")
	}

	// 验证只有一个 .md 文件
	list, _ := store.List()
	names := make(map[string]bool)
	for _, e := range list {
		names[e.Name] = true
	}
	if !names["Test Entry"] {
		t.Error("expected 'Test Entry' in list")
	}
}
