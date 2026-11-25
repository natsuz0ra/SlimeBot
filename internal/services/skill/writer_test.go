package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func learnedMarkdown(name, description, body string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
}

func TestSkillWriterCreateUpdateAndRejectSensitiveOutput(t *testing.T) {
	root := t.TempDir()
	runtime := NewSkillRuntimeService(NewFileSystemSkillStore(root), root)
	writer := newSkillWriter(runtime)

	created, err := writer.write("repeatable-fix", nil, func(current string) (string, error) {
		if current != "" {
			t.Fatal("new skill unexpectedly had current content")
		}
		return learnedMarkdown("repeatable-fix", "Initial procedure", "1. Reproduce the issue.\n2. Apply the verified fix."), nil
	})
	if err != nil || created == nil || created.ID != "repeatable-fix" {
		t.Fatalf("create failed: skill=%#v err=%v", created, err)
	}
	beforeKey := runtime.ToolCacheKey()
	updated, err := writer.write("repeatable-fix", nil, func(current string) (string, error) {
		if !strings.Contains(current, "Reproduce the issue") {
			t.Fatalf("generator did not receive current body: %q", current)
		}
		return learnedMarkdown("repeatable-fix", "Updated procedure", "1. Reproduce the issue.\n2. Apply the verified fix.\n3. Run the regression test."), nil
	})
	if err != nil || updated.Description != "Updated procedure" {
		t.Fatalf("update failed: skill=%#v err=%v", updated, err)
	}
	if runtime.ToolCacheKey() == beforeKey {
		t.Fatal("content-derived tool cache key did not change")
	}
	prompt, _, err := runtime.BuildCatalogPrompt()
	if err != nil || !strings.Contains(prompt, "Updated procedure") {
		t.Fatalf("catalog retained stale content: %q err=%v", prompt, err)
	}

	before, _ := os.ReadFile(filepath.Join(root, "repeatable-fix", "SKILL.md"))
	for _, generated := range []string{
		learnedMarkdown("repeatable-fix", "Bad", "api_key = abcdefghijklmnop"),
		learnedMarkdown("repeatable-fix", "Bad", "Retain {{SB_SECRET_1}} in the procedure."),
		learnedMarkdown("repeatable-fix", "Bad", "session-specific-identifier"),
		learnedMarkdown("other-name", "Bad", "Wrong target."),
		learnedMarkdown("repeatable-fix", "Updated procedure", strings.TrimSpace(strings.SplitN(string(before), "---\n\n", 2)[1])),
	} {
		if _, err := writer.write("repeatable-fix", []string{"session-specific-identifier"}, func(string) (string, error) {
			return generated, nil
		}); err == nil {
			t.Fatalf("expected invalid output to be rejected: %q", generated)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, "repeatable-fix", "SKILL.md"))
	if string(after) != string(before) {
		t.Fatal("rejected generation replaced the previous SKILL.md")
	}
}

func TestSkillWriterSerializesGenerationAndReadBeforeWrite(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "shared", "Shared")
	runtime := NewSkillRuntimeService(NewFileSystemSkillStore(root), root)
	writer := newSkillWriter(runtime)

	const writes = 8
	var wg sync.WaitGroup
	errs := make(chan error, writes)
	for i := 0; i < writes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := writer.write("shared", nil, func(current string) (string, error) {
				return strings.TrimSpace(current) + fmt.Sprintf("\nVerified step %d.", i), nil
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "shared", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < writes; i++ {
		if !strings.Contains(string(raw), fmt.Sprintf("Verified step %d.", i)) {
			t.Fatalf("lost concurrent update %d: %s", i, raw)
		}
	}
}

func TestSkillWriterRejectsSymlinkedSkillDirectory(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	writeSkill(t, external, "linked", "External")
	if err := os.Symlink(filepath.Join(external, "linked"), filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runtime := NewSkillRuntimeService(NewFileSystemSkillStore(root), root)
	writer := newSkillWriter(runtime)
	if _, err := writer.write("linked", nil, func(string) (string, error) {
		return learnedMarkdown("linked", "Changed", "Do not write through links."), nil
	}); err == nil {
		t.Fatal("symlinked skill directory accepted")
	}
}
