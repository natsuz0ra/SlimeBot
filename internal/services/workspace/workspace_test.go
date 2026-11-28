package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkspaceFile(t *testing.T, root, path string, content []byte) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceFileAndTree(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, "src/main.go", []byte("package workspace\n"))
	writeWorkspaceFile(t, root, "data.bin", []byte{0, 1, 2})
	writeWorkspaceFile(t, root, "large.txt", []byte(strings.Repeat("x", MaxPreviewBytes+2)))
	writeWorkspaceFile(t, root, ".gitignore", []byte("node_modules"))
	writeWorkspaceFile(t, root, "node_modules/example", []byte("example"))
	tree, err := OpenTree(root, "", false)
	if err != nil || len(tree) != 3 || tree[0].Name != "src" || !tree[0].IsDirectory {
		t.Fatalf("tree = %+v, %v", tree, err)
	}
	tree, err = OpenTree(root, ".", true)
	if err != nil || len(tree) != 5 {
		t.Fatalf("hidden tree = %+v, %v", tree, err)
	}
	for _, tt := range []struct {
		path              string
		language          string
		binary, truncated bool
	}{
		{"src/main.go", "go", false, false},
		{"data.bin", "", true, false},
		{"large.txt", "", false, true},
	} {
		preview, err := ReadFile(root, tt.path)
		if err != nil || preview.Language != tt.language || preview.Binary != tt.binary || preview.Truncated != tt.truncated ||
			(tt.binary && preview.Content != "") || (tt.truncated && len(preview.Content) != MaxPreviewBytes) {
			t.Fatalf("file %s: language=%q binary=%t truncated=%t len=%d: %v", tt.path, preview.Language, preview.Binary, preview.Truncated, len(preview.Content), err)
		}
	}
	for _, path := range []string{"../outside", "src/../../outside", "..\\outside", "/outside", "C:/outside", filepath.ToSlash(root)} {
		if _, err := ReadFile(root, path); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("accepted %q: %v", path, err)
		}
		if _, err := Diff(context.Background(), root, path, false); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("diff accepted %q: %v", path, err)
		}
	}
}

func TestWorkspaceRejectsEscapingSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeWorkspaceFile(t, outside, "secret.txt", []byte("outside"))
	writeWorkspaceFile(t, root, "inside.txt", []byte("inside"))
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	if err := os.Symlink("inside.txt", filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	if file, err := ReadFile(root, "inside-link"); err != nil || file.Content != "inside" {
		t.Fatalf("safe link: %+v %v", file, err)
	}
	if _, err := ReadFile(root, "escape/secret.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("escaping preview: %v", err)
	}
	if _, err := OpenTree(root, "escape", true); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("escaping tree: %v", err)
	}
	if _, err := Diff(context.Background(), root, "escape/missing.txt", false); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("escaping deleted path: %v", err)
	}
}

func TestGitWorkspaceStatusAndDiffInSubdirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	ctx := context.Background()
	plain, err := Status(ctx, t.TempDir())
	if err != nil || plain.IsRepo {
		t.Fatalf("plain directory: %+v %v", plain, err)
	}
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		git := exec.Command("git", append([]string{"-c", "user.name=Workspace", "-c", "user.email=workspace@example.test", "-c", "core.autocrlf=false"}, args...)...)
		git.Dir = root
		if output, err := git.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, output, err)
		}
	}
	git("init", "-b", "main")
	writeWorkspaceFile(t, root, "project/a file.txt", []byte("before\n"))
	writeWorkspaceFile(t, root, "project/delete.txt", []byte("old\n"))
	writeWorkspaceFile(t, root, "other/outside.txt", []byte("other\n"))
	git("add", ".")
	git("commit", "-m", "base")
	writeWorkspaceFile(t, root, "project/a file.txt", []byte("staged\n"))
	git("add", "project/a file.txt")
	writeWorkspaceFile(t, root, "project/a file.txt", []byte("unstaged\n"))
	if err := os.Remove(filepath.Join(root, "project", "delete.txt")); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceFile(t, root, "project/new.txt", []byte("new without newline"))
	writeWorkspaceFile(t, root, "project/binary.bin", []byte{0, 1, 2})
	writeWorkspaceFile(t, root, "other/outside.txt", []byte("changed outside\n"))
	sub := filepath.Join(root, "project")
	status, err := Status(ctx, sub)
	if err != nil || status.Branch != "main" || len(status.Changes) != 4 || status.Changes[0].Path != "a file.txt" || status.Changes[0].Staged != "M" || status.Changes[0].Unstaged != "M" {
		t.Fatalf("status = %+v, %v", status, err)
	}
	for _, tt := range []struct {
		path   string
		staged bool
		text   string
	}{
		{"a file.txt", true, "+staged"},
		{"a file.txt", false, "+unstaged"},
		{"delete.txt", false, "-old"},
		{"new.txt", false, "\\ No newline at end of file"},
	} {
		diff, err := Diff(ctx, sub, tt.path, tt.staged)
		if err != nil || !strings.Contains(diff.Diff, tt.text) {
			t.Errorf("diff %q staged=%t: %+v, %v", tt.path, tt.staged, diff, err)
		}
	}
	diff, err := Diff(ctx, sub, "binary.bin", false)
	if err != nil || !diff.Binary || diff.Diff != "" {
		t.Fatalf("binary diff = %+v, %v", diff, err)
	}
}

func TestStatusLimitsChangesAndParsesRenames(t *testing.T) {
	var test strings.Builder
	test.WriteString("# branch.ab +3 -2\x002 R. N... 100644 100644 100644 old new R100 nested/new name\x00nested/old name\x00")
	for n := range maxGitChanges {
		fmt.Fprintf(&test, "? nested/file-%d\x00", n)
	}
	status := parseStatus(test.String(), "nested/")
	if len(status.Changes) != maxGitChanges || !status.Truncated || status.Ahead != 3 || status.Behind != 2 ||
		status.Changes[0].OldPath != "old name" || status.Changes[0].Path != "new name" {
		t.Fatalf("invalid parsed status %+v", status.Changes[:min(2, len(status.Changes))])
	}
}
