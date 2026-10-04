package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@localhost", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@localhost")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}
func testRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testGit(t, dir, "init")
	writeTest(t, dir, "a.txt", "original\n")
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "base")
	return dir
}
func writeTest(t *testing.T, dir, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestDirtyBaselineIntegrationPreservesIndex(t *testing.T) {
	root := testRepo(t)
	writeTest(t, root, "a.txt", "staged\n")
	testGit(t, root, "add", "a.txt")
	writeTest(t, root, "dirty.txt", "user's untracked work\n")
	index := testGit(t, root, "diff", "--cached", "--binary")
	m := New(t.TempDir())
	ctx := context.Background()
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	copied, _ := os.ReadFile(filepath.Join(a.Workspace, "dirty.txt"))
	if string(copied) != "user's untracked work\n" {
		t.Fatal("dirty baseline lost")
	}
	writeTest(t, a.Workspace, "a.txt", "staged\nagent change\n")
	writeTest(t, a.Workspace, "new.txt", "new file\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err = m.Integrate(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.Status != "integrated" {
		t.Fatal(a.Status)
	}
	if testGit(t, root, "diff", "--cached", "--binary") != index {
		t.Fatal("parent index changed")
	}
	if err = m.Integrate(ctx, a); err != nil {
		t.Fatalf("retry was not idempotent: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(content) != "staged\nagent change\n" {
		t.Fatal(string(content))
	}
	completed, err := m.Recover(ctx)
	if err != nil || len(completed) != 1 {
		t.Fatalf("recovery %v %v", completed, err)
	}
}
func TestConflictLeavesParentUntouched(t *testing.T) {
	root := testRepo(t)
	m := New(t.TempDir())
	ctx := context.Background()
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, a.Workspace, "a.txt", "agent version\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "a.txt", "human version\n")
	if err = m.Integrate(ctx, a); err == nil {
		t.Fatal("expected conflict")
	}
	actual, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if string(actual) != "human version\n" {
		t.Fatal("conflict modified parent")
	}
}
func TestSymlinkParentCannotEscapeWorkspace(t *testing.T) {
	root := testRepo(t)
	m := New(t.TempDir())
	ctx := context.Background()
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, a.Workspace, "dir/new.txt", "new\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(root, "dir")); err != nil {
		t.Skip(err)
	}
	if err = m.Integrate(ctx, a); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err = os.Stat(filepath.Join(outside, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("wrote outside workspace")
	}
}
func TestCrashJournalRollbackPreservesUnknownEdits(t *testing.T) {
	root := testRepo(t)
	m := New(t.TempDir())
	j := journal{Files: []journalFile{{Path: "a.txt", Before: []byte("original\n"), After: []byte("applied\n"), Mode: 0644}}}
	j.Artifact.ID = "test"
	j.Artifact.ParentWorkspace = root
	writeTest(t, root, "a.txt", "applied\n")
	if err := writeJournal(m.journalPath("test"), j); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(root, "a.txt"))
	if !bytes.Equal(content, j.Files[0].Before) {
		t.Fatal("rollback failed")
	}
	writeTest(t, root, "a.txt", "subsequent user edit\n")
	_ = writeJournal(m.journalPath("test"), j)
	if _, err := m.Recover(context.Background()); err == nil {
		t.Fatal("unknown edit overwritten")
	}
}

func TestCaptureInvalidatesOnlyChangedResult(t *testing.T) {
	ctx := context.Background()
	m := New(t.TempDir())
	a, err := m.Create(ctx, testRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, a.Workspace, "new.txt", "result\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.ValidatedCommit, a.ValidationCommand = a.ResultCommit, "test -f new.txt"
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.ValidatedCommit != a.ResultCommit {
		t.Fatal("unchanged result invalidated")
	}
	writeTest(t, a.Workspace, "new.txt", "modified\n")
	matches, err := m.MatchesResult(ctx, *a)
	if err != nil || matches {
		t.Fatalf("undetected source change: %v %v", matches, err)
	}
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.ValidatedCommit != "" || a.ValidationCommand != "" {
		t.Fatal("changed result kept stale validation")
	}
}

func TestMergedValidationFailurePreservesParent(t *testing.T) {
	ctx := context.Background()
	root := testRepo(t)
	m := New(t.TempDir())
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, a.Workspace, "new.txt", "agent\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "user.txt", "user\n")
	m.ValidatePreview = func(ctx context.Context, preview, command string) error {
		if _, err := os.Stat(filepath.Join(preview, "user.txt")); err != nil {
			t.Fatal("parent edits absent from preview")
		}
		if _, err := os.Stat(filepath.Join(preview, "new.txt")); err != nil {
			t.Fatal("agent result absent from preview")
		}
		return errors.New("combined result fails")
	}
	if err = m.Integrate(ctx, a); err == nil {
		t.Fatal("expected validation failure")
	}
	if a.Status != "validation_failed" {
		t.Fatal(a.Status)
	}
	if _, err = os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("parent changed on failed validation")
	}
}

func TestValidationCannotChangeMergedSource(t *testing.T) {
	ctx := context.Background()
	root := testRepo(t)
	m := New(t.TempDir())
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, a.Workspace, "new.txt", "agent\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	m.ValidatePreview = func(ctx context.Context, preview, command string) error {
		writeTest(t, preview, "new.txt", "test modified source\n")
		return nil
	}
	if err = m.Integrate(ctx, a); err == nil {
		t.Fatal("test source mutation was accepted")
	}
	if _, err = os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("parent changed")
	}
}

func TestDirectoryWorkspaceIntegratesWithoutCreatingUserGitMetadata(t *testing.T) {
	root := t.TempDir()
	writeTest(t, root, "a.txt", "original\n")
	writeTest(t, root, ".gitignore", "ignored.txt\n")
	writeTest(t, root, "ignored.txt", "local data\n")
	m := New(t.TempDir())
	ctx := context.Background()
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if a.GitDirectory == "" {
		t.Fatal("ordinary directory has no managed Git database")
	}
	if _, err = os.Stat(filepath.Join(a.Workspace, "ignored.txt")); !os.IsNotExist(err) {
		t.Fatal("ignored data copied")
	}
	writeTest(t, a.Workspace, "new.txt", "agent\n")
	if err = m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	writeTest(t, root, "user.txt", "user\n")
	if err = m.Integrate(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("created .git in user's directory")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "new.txt")); string(b) != "agent\n" {
		t.Fatal("agent file missing")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "user.txt")); string(b) != "user\n" {
		t.Fatal("user edits lost")
	}
	if err = m.Archive(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Diff(ctx, *a); err != nil {
		t.Fatal("archived snapshot unavailable:", err)
	}
}

func TestSnapshotPreservesStagedDeletion(t *testing.T) {
	root := testRepo(t)
	testGit(t, root, "rm", "a.txt")
	m := New(t.TempDir())
	a, err := m.Create(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(a.Workspace, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("staged deletion resurrected")
	}
}

func TestIntegrationEnforcesFrozenWriteScopes(t *testing.T) {
	root := testRepo(t)
	m := New(t.TempDir())
	ctx := context.Background()
	a, err := m.Create(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	a.WriteScopes = `["allowed"]`
	writeTest(t, a.Workspace, "other.txt", "outside\n")
	if err := m.Capture(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := m.Integrate(ctx, a); err == nil {
		t.Fatal("out-of-scope change was integrated")
	}
	if _, err := os.Stat(filepath.Join(root, "other.txt")); !os.IsNotExist(err) {
		t.Fatal("parent changed on rejected integration")
	}
	for _, scope := range []string{"../escape", "a/../b", "/absolute", "*.go", "C:/Windows"} {
		if err := ValidateScopes([]string{scope}); err == nil {
			t.Fatalf("accepted scope %q", scope)
		}
	}
}
