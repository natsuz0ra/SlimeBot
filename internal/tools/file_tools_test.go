package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileReadReadsTextWithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	state := NewReadFileState()
	res, err := (&fileReadTool{}).read(WithReadFileState(context.Background(), state), map[string]string{
		"file_path": path,
		"offset":    "2",
		"limit":     "1",
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(res.Output, "Total lines: 3") || !strings.Contains(res.Output, "     2\tbeta") {
		t.Fatalf("unexpected output:\n%s", res.Output)
	}
	entry, ok := state.get(path)
	if !ok {
		t.Fatal("expected read state entry")
	}
	if !entry.Partial {
		t.Fatal("expected partial read state for ranged read")
	}
}

func TestFileReadEmptyFileWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	res, err := (&fileReadTool{}).read(context.Background(), map[string]string{"file_path": path})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(res.Output, "empty") {
		t.Fatalf("expected empty file warning, got:\n%s", res.Output)
	}
}

func TestFileReadRejectsLargeDirectoryMissingAndDevice(t *testing.T) {
	dir := t.TempDir()
	tool := &fileReadTool{}
	if _, err := tool.read(context.Background(), map[string]string{"file_path": dir}); err == nil {
		t.Fatal("expected directory rejection")
	}
	if _, err := tool.read(context.Background(), map[string]string{"file_path": filepath.Join(dir, "missing.txt")}); err == nil {
		t.Fatal("expected missing file rejection")
	}

	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, []byte(strings.Repeat("a", fileReadMaxSizeBytes+1)), 0o644); err != nil {
		t.Fatalf("write large fixture: %v", err)
	}
	if _, err := tool.read(context.Background(), map[string]string{"file_path": large}); err == nil {
		t.Fatal("expected large file rejection")
	}

	if runtime.GOOS != "windows" {
		if _, err := tool.read(context.Background(), map[string]string{"file_path": "/dev/zero"}); err == nil {
			t.Fatal("expected blocked device rejection")
		}
	}
}

func TestFileEditRequiresFreshFullRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := WithReadFileState(context.Background(), NewReadFileState())

	_, err := (&fileEditTool{}).edit(ctx, map[string]string{
		"file_path":  path,
		"old_string": "beta",
		"new_string": "delta",
	})
	if err == nil || !strings.Contains(err.Error(), "fully read") {
		t.Fatalf("expected unread rejection, got %v", err)
	}

	if _, err := (&fileReadTool{}).read(ctx, map[string]string{"file_path": path}); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("alpha\nchanged\n"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}
	_, err = (&fileEditTool{}).edit(ctx, map[string]string{
		"file_path":  path,
		"old_string": "changed",
		"new_string": "delta",
	})
	if err == nil || !strings.Contains(err.Error(), "modified since") {
		t.Fatalf("expected stale rejection, got %v", err)
	}
}

func TestFileEditUniqueReplaceAllAndCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("one two two\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := WithReadFileState(context.Background(), NewReadFileState())
	if _, err := (&fileReadTool{}).read(ctx, map[string]string{"file_path": path}); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if _, err := (&fileEditTool{}).edit(ctx, map[string]string{
		"file_path":  path,
		"old_string": "two",
		"new_string": "three",
	}); err == nil || !strings.Contains(err.Error(), "matches") {
		t.Fatalf("expected multiple match rejection, got %v", err)
	}

	if _, err := (&fileEditTool{}).edit(ctx, map[string]string{
		"file_path":   path,
		"old_string":  "two",
		"new_string":  "three",
		"replace_all": "true",
	}); err != nil {
		t.Fatalf("replace_all edit failed: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "one three three\n" {
		t.Fatalf("unexpected edited content: %q", got)
	}

	newPath := filepath.Join(dir, "nested", "created.txt")
	if _, err := (&fileEditTool{}).edit(ctx, map[string]string{
		"file_path":  newPath,
		"old_string": "",
		"new_string": "created\n",
	}); err != nil {
		t.Fatalf("create edit failed: %v", err)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "created\n" {
		t.Fatalf("unexpected created content: %q", got)
	}
}

func TestFileEditRejectsBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary.bin")
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0x00}, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := WithReadFileState(context.Background(), NewReadFileState())
	state := readFileStateFromContext(ctx)
	info, _ := os.Stat(path)
	state.set(path, ReadFileEntry{Content: "", MTimeUnix: fileMTimeUnix(info), Partial: false})
	if _, err := (&fileEditTool{}).edit(ctx, map[string]string{
		"file_path":  path,
		"old_string": "x",
		"new_string": "y",
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "binary") {
		t.Fatalf("expected binary/utf8 rejection, got %v", err)
	}
}

func TestFileWriteCreateOverwriteAndPartialReadRejection(t *testing.T) {
	dir := t.TempDir()
	ctx := WithReadFileState(context.Background(), NewReadFileState())
	tool := &fileWriteTool{}

	newPath := filepath.Join(dir, "nested", "new.txt")
	if _, err := tool.write(ctx, map[string]string{"file_path": newPath, "content": "hello\n"}); err != nil {
		t.Fatalf("create write failed: %v", err)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "hello\n" {
		t.Fatalf("unexpected created content: %q", got)
	}

	if _, err := tool.write(ctx, map[string]string{"file_path": newPath, "content": "overwrite\n"}); err != nil {
		t.Fatalf("overwrite after state update failed: %v", err)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "overwrite\n" {
		t.Fatalf("unexpected overwritten content: %q", got)
	}

	partial := filepath.Join(dir, "partial.txt")
	if err := os.WriteFile(partial, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := (&fileReadTool{}).read(ctx, map[string]string{"file_path": partial, "offset": "2", "limit": "1"}); err != nil {
		t.Fatalf("partial read failed: %v", err)
	}
	if _, err := tool.write(ctx, map[string]string{"file_path": partial, "content": "full\n"}); err == nil || !strings.Contains(err.Error(), "fully read") {
		t.Fatalf("expected partial read rejection, got %v", err)
	}
}
