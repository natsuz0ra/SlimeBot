package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSystemSkillStore_ListSkillsReadsDirectory(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha skill")
	writeSkill(t, root, "beta", "Beta skill")

	store := NewFileSystemSkillStore(root)
	items, err := store.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(items))
	}
	if items[0].ID == "" || items[0].RelativePath == "" {
		t.Fatalf("expected stable metadata, got %#v", items[0])
	}
}

func TestFileSystemSkillStore_GetSkillByName(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha skill")

	store := NewFileSystemSkillStore(root)
	item, err := store.GetSkillByName("alpha")
	if err != nil {
		t.Fatalf("GetSkillByName failed: %v", err)
	}
	if item == nil || item.Name != "alpha" || item.Description != "Alpha skill" {
		t.Fatalf("unexpected skill: %#v", item)
	}
}

func TestFileSystemSkillStore_DeleteSkillRemovesDirectory(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "alpha", "Alpha skill")

	store := NewFileSystemSkillStore(root)
	if err := store.DeleteSkill("alpha"); err != nil {
		t.Fatalf("DeleteSkill failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("expected directory deleted, stat err=%v", err)
	}
}

func TestSkillRuntimeService_BuildCatalogPromptUsesDirectoryStore(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "beta", "Beta skill")
	writeSkill(t, root, "alpha", "Alpha skill")

	store := NewFileSystemSkillStore(root)
	svc := NewSkillRuntimeService(store, root)
	prompt, skills, err := svc.BuildCatalogPrompt()
	if err != nil {
		t.Fatalf("BuildCatalogPrompt failed: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	if !strings.Contains(prompt, "alpha") || !strings.Contains(prompt, "Alpha skill") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}
	if skills[0].Name != "alpha" || skills[1].Name != "beta" {
		t.Fatalf("expected sorted skills [alpha beta], got [%s %s]", skills[0].Name, skills[1].Name)
	}
	if strings.Index(prompt, "<name>alpha</name>") > strings.Index(prompt, "<name>beta</name>") {
		t.Fatalf("expected alpha before beta in prompt: %s", prompt)
	}
}

func writeSkill(t *testing.T, root, name, desc string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill failed: %v", err)
	}
}
