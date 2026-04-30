package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type fileEditTool struct{}

func init() {
	Register(&fileEditTool{})
}

func (f *fileEditTool) Name() string { return "file_edit" }

func (f *fileEditTool) Description() string {
	return "Edit UTF-8 text files by replacing exact strings, with stale-write protection."
}

func (f *fileEditTool) Commands() []Command {
	return []Command{{
		Name:        "edit",
		Description: "Replace exact text in a UTF-8 file. Existing files must be fully read with file_read first; if the file changed since that read, the edit is rejected. Use old_string=\"\" only to create or fill an empty file.",
		Params: []CommandParam{
			{Name: "file_path", Required: true, Description: "Absolute or relative path to the file to edit.", Example: "/path/to/file.go"},
			{Name: "old_string", Required: true, Description: "Exact text to replace. Use an empty string only for creating a new file or replacing an empty file.", Example: "old text"},
			{Name: "new_string", Required: true, Description: "Replacement text. Must differ from old_string.", Example: "new text"},
			{Name: "replace_all", Required: false, Description: "Set to true to replace every occurrence. Defaults to false and requires old_string to match exactly once.", Example: "false"},
		},
	}}
}

func (f *fileEditTool) Execute(ctx context.Context, command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "edit":
		return f.edit(ctx, params)
	default:
		return nil, fmt.Errorf("file_edit tool does not support command: %s", command)
	}
}

func (f *fileEditTool) edit(ctx context.Context, params map[string]string) (*ExecuteResult, error) {
	path, err := resolveFilePath(params["file_path"])
	if err != nil {
		return nil, err
	}
	oldString := params["old_string"]
	newString := params["new_string"]
	if oldString == newString {
		return nil, fmt.Errorf("old_string and new_string are exactly the same")
	}
	replaceAll, err := parseBoolParam(params["replace_all"])
	if err != nil {
		return nil, err
	}

	info, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("failed to stat file: %w", statErr)
	}
	if statErr == nil && info.IsDir() {
		return nil, fmt.Errorf("file_path is a directory, not a file: %s", path)
	}

	var original string
	var existed bool
	if os.IsNotExist(statErr) {
		if oldString != "" {
			return nil, fmt.Errorf("file does not exist; use old_string=\"\" to create a new file")
		}
		original = ""
	} else {
		existed = true
		if info.Size() > fileWriteMaxSizeBytes {
			return nil, fmt.Errorf("file is too large to edit (%d bytes, max %d)", info.Size(), fileWriteMaxSizeBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		original, err = validateTextBytes(data)
		if err != nil {
			return nil, err
		}
		if err := requireFreshFullRead(ctx, path, original, info); err != nil {
			return nil, err
		}
	}

	if oldString == "" && existed && strings.TrimSpace(original) != "" {
		return nil, fmt.Errorf("cannot create new file: file already exists and is not empty")
	}
	actualNew := normalizeReplacementLineEndings(original, newString)
	updated, count, err := applyTextEdit(original, oldString, actualNew, replaceAll)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	newInfo, _ := os.Stat(path)
	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{Content: updated, MTimeUnix: fileMTimeUnix(newInfo), Offset: 1, Limit: 0, Partial: false})
	}

	action := "updated"
	if !existed {
		action = "created"
	}
	return &ExecuteResult{Output: fmt.Sprintf("File %s successfully: %s\nReplacements: %d", action, path, count)}, nil
}

func parseBoolParam(raw string) (bool, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("replace_all must be true or false")
	}
	return value, nil
}

func requireFreshFullRead(ctx context.Context, path string, current string, info os.FileInfo) error {
	state := readFileStateFromContext(ctx)
	if state == nil {
		return fmt.Errorf("file has not been read yet. Read it first with file_read before writing to it")
	}
	entry, ok := state.get(path)
	if !ok || entry.Partial {
		return fmt.Errorf("file has not been fully read yet. Read the full file with file_read before writing to it")
	}
	if current != entry.Content {
		return fmt.Errorf("file has been modified since it was read. Read it again before attempting to write")
	}
	if fileMTimeUnix(info) > entry.MTimeUnix && current != entry.Content {
		return fmt.Errorf("file has been modified since it was read. Read it again before attempting to write")
	}
	return nil
}

func applyTextEdit(original, oldString, newString string, replaceAll bool) (string, int, error) {
	if oldString == "" {
		if original != "" {
			return "", 0, fmt.Errorf("old_string is empty but file is not empty")
		}
		if newString == "" {
			return "", 0, fmt.Errorf("new_string is empty; no changes to make")
		}
		return newString, 1, nil
	}
	count := strings.Count(original, oldString)
	if count == 0 {
		return "", 0, fmt.Errorf("string to replace was not found in file")
	}
	if count > 1 && !replaceAll {
		return "", 0, fmt.Errorf("found %d matches of old_string; provide more context or set replace_all=true", count)
	}
	if replaceAll {
		return strings.ReplaceAll(original, oldString, newString), count, nil
	}
	return strings.Replace(original, oldString, newString, 1), 1, nil
}
