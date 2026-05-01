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
	res, err := (&fileReadTool{}).read(WithReadFileState(context.Background(), state), map[string]any{
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
	res, err := (&fileReadTool{}).read(context.Background(), map[string]any{"file_path": path})
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
	if _, err := tool.read(context.Background(), map[string]any{"file_path": dir}); err == nil {
		t.Fatal("expected directory rejection")
	}
	if _, err := tool.read(context.Background(), map[string]any{"file_path": filepath.Join(dir, "missing.txt")}); err == nil {
		t.Fatal("expected missing file rejection")
	}

	large := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(large, []byte(strings.Repeat("a", fileReadMaxSizeBytes+1)), 0o644); err != nil {
		t.Fatalf("write large fixture: %v", err)
	}
	if _, err := tool.read(context.Background(), map[string]any{"file_path": large}); err == nil {
		t.Fatal("expected large file rejection")
	}

	if runtime.GOOS != "windows" {
		if _, err := tool.read(context.Background(), map[string]any{"file_path": "/dev/zero"}); err == nil {
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

	_, err := (&fileEditTool{}).edit(ctx, map[string]any{
		"file_path":  path,
		"old_string": "beta",
		"new_string": "delta",
	})
	if err == nil || !strings.Contains(err.Error(), "fully read") {
		t.Fatalf("expected unread rejection, got %v", err)
	}

	if _, err := (&fileReadTool{}).read(ctx, map[string]any{"file_path": path}); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("alpha\nchanged\n"), 0o644); err != nil {
		t.Fatalf("external write: %v", err)
	}
	_, err = (&fileEditTool{}).edit(ctx, map[string]any{
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
	if _, err := (&fileReadTool{}).read(ctx, map[string]any{"file_path": path}); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if _, err := (&fileEditTool{}).edit(ctx, map[string]any{
		"file_path":  path,
		"old_string": "two",
		"new_string": "three",
	}); err == nil || !strings.Contains(err.Error(), "matches") {
		t.Fatalf("expected multiple match rejection, got %v", err)
	}

	res, err := (&fileEditTool{}).edit(ctx, map[string]any{
		"file_path":   path,
		"old_string":  "two",
		"new_string":  "three",
		"replace_all": "true",
	})
	if err != nil {
		t.Fatalf("replace_all edit failed: %v", err)
	}
	metadata, ok := res.Metadata.(FileToolMetadata)
	if !ok {
		t.Fatalf("expected file metadata, got %#v", res.Metadata)
	}
	if metadata.Operation != "Update" || metadata.Summary != "Updated sample.txt" {
		t.Fatalf("unexpected metadata summary: %+v", metadata)
	}
	if !hasDiffLine(metadata.DiffLines, "removed", 1, 0, "one two two") || !hasDiffLine(metadata.DiffLines, "added", 0, 1, "one three three") {
		t.Fatalf("expected replace_all diff lines, got %+v", metadata.DiffLines)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "one three three\n" {
		t.Fatalf("unexpected edited content: %q", got)
	}

	newPath := filepath.Join(dir, "nested", "created.txt")
	res, err = (&fileEditTool{}).edit(ctx, map[string]any{
		"file_path":  newPath,
		"old_string": "",
		"new_string": "created\n",
	})
	if err != nil {
		t.Fatalf("create edit failed: %v", err)
	}
	metadata, ok = res.Metadata.(FileToolMetadata)
	if !ok || metadata.Operation != "Create" || metadata.Summary != "Created created.txt" {
		t.Fatalf("unexpected create metadata: %#v", res.Metadata)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "created\n" {
		t.Fatalf("unexpected created content: %q", got)
	}
}

func TestFileEditMetadataIncludesNearbyContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := strings.Join([]string{
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ctx := WithReadFileState(context.Background(), NewReadFileState())
	if _, err := (&fileReadTool{}).read(ctx, map[string]any{"file_path": path}); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	res, err := (&fileEditTool{}).edit(ctx, map[string]any{
		"file_path":  path,
		"old_string": "line 5",
		"new_string": "changed 5",
	})
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}
	metadata, ok := res.Metadata.(FileToolMetadata)
	if !ok {
		t.Fatalf("expected file metadata, got %#v", res.Metadata)
	}
	if len(metadata.DiffLines) != 8 {
		t.Fatalf("expected 3 context lines around one change, got %+v", metadata.DiffLines)
	}
	if metadata.DiffLines[0].Text != "line 2" || metadata.DiffLines[len(metadata.DiffLines)-1].Text != "line 8" {
		t.Fatalf("unexpected context window: %+v", metadata.DiffLines)
	}
	if !hasDiffLine(metadata.DiffLines, "removed", 5, 0, "line 5") || !hasDiffLine(metadata.DiffLines, "added", 0, 5, "changed 5") {
		t.Fatalf("missing changed lines: %+v", metadata.DiffLines)
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
	if _, err := (&fileEditTool{}).edit(ctx, map[string]any{
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
	res, err := tool.write(ctx, map[string]any{"file_path": newPath, "content": "hello\n"})
	if err != nil {
		t.Fatalf("create write failed: %v", err)
	}
	metadata, ok := res.Metadata.(FileToolMetadata)
	if !ok || metadata.Operation != "Create" || metadata.Summary != "Created new.txt" {
		t.Fatalf("unexpected create write metadata: %#v", res.Metadata)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "hello\n" {
		t.Fatalf("unexpected created content: %q", got)
	}

	res, err = tool.write(ctx, map[string]any{"file_path": newPath, "content": "overwrite\n"})
	if err != nil {
		t.Fatalf("overwrite after state update failed: %v", err)
	}
	metadata, ok = res.Metadata.(FileToolMetadata)
	if !ok || metadata.Operation != "Write" || metadata.Summary != "Wrote new.txt" {
		t.Fatalf("unexpected overwrite metadata: %#v", res.Metadata)
	}
	if !hasDiffLine(metadata.DiffLines, "removed", 1, 0, "hello") || !hasDiffLine(metadata.DiffLines, "added", 0, 1, "overwrite") {
		t.Fatalf("unexpected overwrite diff: %+v", metadata.DiffLines)
	}
	if got, _ := os.ReadFile(newPath); string(got) != "overwrite\n" {
		t.Fatalf("unexpected overwritten content: %q", got)
	}

	partial := filepath.Join(dir, "partial.txt")
	if err := os.WriteFile(partial, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := (&fileReadTool{}).read(ctx, map[string]any{"file_path": partial, "offset": "2", "limit": "1"}); err != nil {
		t.Fatalf("partial read failed: %v", err)
	}
	if _, err := tool.write(ctx, map[string]any{"file_path": partial, "content": "full\n"}); err == nil || !strings.Contains(err.Error(), "fully read") {
		t.Fatalf("expected partial read rejection, got %v", err)
	}
}

func hasDiffLine(lines []FileDiffLine, kind string, oldLine int, newLine int, text string) bool {
	for _, line := range lines {
		if line.Kind != kind || line.Text != text {
			continue
		}
		if oldLine > 0 {
			if line.OldLine == nil || *line.OldLine != oldLine {
				continue
			}
		}
		if newLine > 0 {
			if line.NewLine == nil || *line.NewLine != newLine {
				continue
			}
		}
		return true
	}
	return false
}
