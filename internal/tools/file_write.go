package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type fileWriteTool struct{}

func init() {
	Register(&fileWriteTool{})
}

func (f *fileWriteTool) Name() string { return "file_write" }

func (f *fileWriteTool) Description() string {
	return "Create or fully overwrite UTF-8 text files, with stale-write protection for existing files."
}

func (f *fileWriteTool) Commands() []Command {
	return []Command{{
		Name:        "write",
		Description: "Create a UTF-8 text file or fully overwrite an existing file. Existing files must be fully read with file_read first; prefer file_edit for small modifications.",
		Params: []CommandParam{
			{Name: "file_path", Required: true, Description: "Absolute or relative path to write.", Example: "/path/to/file.go"},
			{Name: "content", Required: true, Description: "Complete UTF-8 text content to write to the file.", Example: "package main\n"},
		},
	}}
}

func (f *fileWriteTool) Execute(ctx context.Context, command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "write":
		return f.write(ctx, params)
	default:
		return nil, fmt.Errorf("file_write tool does not support command: %s", command)
	}
}

func (f *fileWriteTool) write(ctx context.Context, params map[string]string) (*ExecuteResult, error) {
	path, err := resolveFilePath(params["file_path"])
	if err != nil {
		return nil, err
	}
	content, err := validateTextBytes([]byte(params["content"]))
	if err != nil {
		return nil, err
	}
	if len([]byte(content)) > fileWriteMaxSizeBytes {
		return nil, fmt.Errorf("content is too large to write (%d bytes, max %d)", len([]byte(content)), fileWriteMaxSizeBytes)
	}

	info, statErr := os.Stat(path)
	existed := false
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("failed to stat file: %w", statErr)
	}
	var original string
	if statErr == nil {
		if info.IsDir() {
			return nil, fmt.Errorf("file_path is a directory, not a file: %s", path)
		}
		existed = true
		if info.Size() > fileWriteMaxSizeBytes {
			return nil, fmt.Errorf("file is too large to overwrite (%d bytes, max %d)", info.Size(), fileWriteMaxSizeBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing file: %w", err)
		}
		original, err = validateTextBytes(data)
		if err != nil {
			return nil, err
		}
		if err := requireFreshFullRead(ctx, path, original, info); err != nil {
			return nil, err
		}
		content = normalizeReplacementLineEndings(original, content)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	newInfo, _ := os.Stat(path)
	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{Content: content, MTimeUnix: fileMTimeUnix(newInfo), Offset: 1, Limit: 0, Partial: false})
	}

	if existed {
		return &ExecuteResult{Output: fmt.Sprintf("File updated successfully: %s\nBytes written: %d", path, len([]byte(content)))}, nil
	}
	return &ExecuteResult{Output: fmt.Sprintf("File created successfully: %s\nBytes written: %d", path, len([]byte(content)))}, nil
}
