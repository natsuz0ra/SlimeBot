package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkpointFixture(t *testing.T) (*CheckpointManager, string) {
	t.Helper()
	root, err := ResolveCheckpointRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewCheckpoints(t.TempDir()), root
}

func checkpointWrite(t *testing.T, root, path, content string) {
	t.Helper()
	name := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertCheckpointFile(t *testing.T, root, path, content string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || string(got) != content {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, content)
	}
}

func captureCheckpoint(t *testing.T, m *CheckpointManager, root string, edit func()) Checkpoint {
	t.Helper()
	ctx := context.Background()
	capture, err := m.BeginCapture(ctx, "session", root, "turn-request", "user-message")
	if err != nil {
		t.Fatal(err)
	}
	edit()
	if err := capture.Finish(ctx, root, false); err != nil {
		t.Fatal(err)
	}
	items, err := m.List("session")
	if err != nil || len(items) == 0 {
		t.Fatalf("invalid history: %+v, %v", items, err)
	}
	return items[0]
}

func TestCheckpointDiffUndoAndRestoreAfterRestart(t *testing.T) {
	ctx := context.Background()
	m, root := checkpointFixture(t)
	checkpointWrite(t, root, "notes.txt", "original\r\n")
	checkpointWrite(t, root, "old.txt", "deleted\n")
	checkpointWrite(t, root, ".env", "sensitive fixture")
	first := captureCheckpoint(t, m, root, func() {
		checkpointWrite(t, root, "notes.txt", "first\r\n")
		checkpointWrite(t, root, "new.txt", "new file\n")
		checkpointWrite(t, root, ".env", "updated sensitive fixture")
		if err := os.Remove(filepath.Join(root, "old.txt")); err != nil {
			t.Fatal(err)
		}
	})
	if first.FileCount != 3 || !first.Limited || first.UserMessageID != "user-message" {
		t.Fatalf("unexpected capture: %+v", first)
	}
	second := captureCheckpoint(t, m, root, func() {
		checkpointWrite(t, root, "notes.txt", "second\r\n")
		checkpointWrite(t, root, "assets/binary", "\x00\x01\x02")
	})
	m = NewCheckpoints(m.directory)
	detail, err := m.Detail("session", first.ID)
	if err != nil || len(detail.Files) != 3 || detail.Files[0].Kind != "added" || detail.Files[2].Kind != "deleted" {
		t.Fatalf("details: %+v / %v", detail, err)
	}
	diff, err := m.Diff("session", first.ID, "notes.txt")
	if err != nil || !strings.Contains(diff.Diff, "-original") || !strings.Contains(diff.Diff, "+first") {
		t.Fatalf("diff: %+v / %v", diff, err)
	}
	binary, err := m.Diff("session", second.ID, "assets/binary")
	if err != nil || !binary.Binary || binary.Diff != "" {
		t.Fatalf("binary diff: %+v / %v", binary, err)
	}
	if _, err := m.Detail("other-session", first.ID); !errors.Is(err, ErrCheckpointMissing) {
		t.Fatalf("cross-session lookup: %v", err)
	}
	if err := m.Rollback(ctx, "session", root, first.ID, "undo", nil); !errors.Is(err, ErrCheckpointConflict) {
		t.Fatalf("undo older change should conflict: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("conflicting rollback partially mutated files: %v", err)
	}
	checkpointWrite(t, root, "unrelated.txt", "keep this edit")
	if err := m.Rollback(ctx, "session", root, first.ID, "restore", nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFile(t, root, "notes.txt", "first\r\n")
	assertCheckpointFile(t, root, "unrelated.txt", "keep this edit")
	if _, err := os.Stat(filepath.Join(root, "assets", "binary")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary file survived restore: %v", err)
	}
	items, err := m.List("session")
	if err != nil || len(items) != 3 || items[0].Kind != "restore" || items[0].SourceID != first.ID {
		t.Fatalf("restore journal: %+v / %v", items, err)
	}
	if err := m.Rollback(ctx, "session", root, first.ID, "undo", nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFile(t, root, "notes.txt", "original\r\n")
	assertCheckpointFile(t, root, "old.txt", "deleted\n")
	assertCheckpointFile(t, root, ".env", "updated sensitive fixture")
	if err := m.DeleteSession("session"); err != nil {
		t.Fatal(err)
	}
	items, err = m.List("session")
	if err != nil || len(items) != 0 {
		t.Fatalf("deleted session history: %+v / %v", items, err)
	}
}

func TestCheckpointPreservesGitIndexAndIgnoredFiles(t *testing.T) {
	ctx := context.Background()
	m, root := checkpointFixture(t)
	if _, err := git(ctx, root, nil, "init"); err != nil {
		t.Skipf("Git unavailable: %v", err)
	}
	checkpointWrite(t, root, ".gitignore", "scratch/\n")
	checkpointWrite(t, root, "sample.txt", "staged\n")
	checkpointWrite(t, root, "scratch/log.txt", "ignored\n")
	if _, err := git(ctx, root, nil, "add", "sample.txt"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	checkpointWrite(t, root, "sample.txt", "dirty\n")
	item := captureCheckpoint(t, m, root, func() {
		checkpointWrite(t, root, "sample.txt", "changed\n")
		checkpointWrite(t, root, "scratch/log.txt", "still ignored\n")
	})
	if item.FileCount != 1 {
		t.Fatalf("expected only sample.txt: %+v", item)
	}
	if err = m.Rollback(ctx, "session", root, item.ID, "undo", nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFile(t, root, "sample.txt", "dirty\n")
	assertCheckpointFile(t, root, "scratch/log.txt", "still ignored\n")
	after, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("checkpoint modified the Git index: %v", err)
	}
	unignored := captureCheckpoint(t, m, root, func() { checkpointWrite(t, root, ".gitignore", "# no ignored files\n") })
	if unignored.FileCount != 1 || !unignored.Limited {
		t.Fatalf("preexisting ignored file was recorded as new: %+v", unignored)
	}
	if err = m.Rollback(ctx, "session", root, unignored.ID, "undo", nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFile(t, root, "scratch/log.txt", "still ignored\n")
}

func TestCheckpointRecoversInterruptedRollback(t *testing.T) {
	ctx := context.Background()
	m, root := checkpointFixture(t)
	checkpointWrite(t, root, "a.txt", "before a")
	checkpointWrite(t, root, "b.txt", "before b")
	turn := captureCheckpoint(t, m, root, func() {
		checkpointWrite(t, root, "a.txt", "after a")
		checkpointWrite(t, root, "b.txt", "after b")
	})
	original, err := m.record("session", turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := m.newRecord("session", root, "undo")
	if err != nil {
		t.Fatal(err)
	}
	pending.changes, err = m.rollbackChanges("session", original, "undo")
	if err != nil {
		t.Fatal(err)
	}
	pending.FileCount, pending.Identity = len(pending.changes), original.Identity
	if err := writeCheckpoint(filepath.Join(m.sessionPath("session"), "pending.json"), pending); err != nil {
		t.Fatal(err)
	}
	checkpointWrite(t, root, "a.txt", "before a") // The process stopped after the first mutation.
	m = NewCheckpoints(m.directory)
	if _, err := m.ReserveRoots(ctx, []string{root}, false, false, "other-session"); !errors.Is(err, ErrCheckpointConflict) {
		t.Fatalf("another session could write over a pending rollback: %v", err)
	}
	if err := m.Recover("session", root); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFile(t, root, "a.txt", "after a")
	assertCheckpointFile(t, root, "b.txt", "after b")
	if err := m.Rollback(ctx, "session", root, turn.ID, "undo", nil); err != nil {
		t.Fatal(err)
	}
	assertCheckpointFile(t, root, "a.txt", "before a")
	assertCheckpointFile(t, root, "b.txt", "before b")
}

func TestCheckpointLeaseAndHistoryGap(t *testing.T) {
	ctx := context.Background()
	m, root := checkpointFixture(t)
	first := captureCheckpoint(t, m, root, func() { checkpointWrite(t, root, "a", "before gap") })
	capture, err := m.BeginCapture(ctx, "session", root, "gap", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := capture.Finish(ctx, root, true); err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback(ctx, "session", root, first.ID, "restore", nil); !errors.Is(err, ErrCheckpointConflict) {
		t.Fatalf("restored across a gap: %v", err)
	}
	a, err := m.Reserve(ctx, root, false, true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Reserve(ctx, filepath.Join(root, "nested"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reserve(ctx, filepath.Join(root, "nested"), false); !errors.Is(err, ErrCheckpointBusy) {
		t.Fatalf("overlapping writer allowed: %v", err)
	}
	b()
	a()
	done, err := m.Reserve(ctx, root, false)
	if err != nil {
		t.Fatal(err)
	}
	done()
}
