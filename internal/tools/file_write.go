package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type fileWriteTool struct{}

type fileWriteRequest struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

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
		Description: "Create or overwrite files. Supports single write and batch writes via writes[].",
		Params: []CommandParam{
			{Name: "file_path", Required: false, Description: "Single-write mode file path.", Example: "/path/to/file.go"},
			{Name: "content", Required: false, Description: "Single-write mode full file content.", Example: "package main\n"},
			{Name: "writes", Required: false, Description: "Batch mode: [{file_path,content}]", Schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"file_path", "content"},
				},
			}},
		},
	}}
}

func (f *fileWriteTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "write":
		return f.write(ctx, params)
	default:
		return nil, fmt.Errorf("file_write tool does not support command: %s", command)
	}
}

func (f *fileWriteTool) write(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	requests, err := parseFileWriteRequests(params)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	metas := make([]FileToolMetadata, 0, len(requests))
	success := 0
	failed := 0
	for i, req := range requests {
		meta, msg, writeErr := f.writeOne(ctx, req)
		if i > 0 {
			out.WriteString("\n")
		}
		if writeErr != nil {
			if len(requests) == 1 {
				return nil, writeErr
			}
			failed++
			out.WriteString(fmt.Sprintf("Write %d failed: %v", i+1, writeErr))
			continue
		}
		success++
		metas = append(metas, meta)
		out.WriteString(msg)
	}
	out.WriteString(fmt.Sprintf("\nSummary: succeeded=%d failed=%d total=%d", success, failed, len(requests)))
	if len(requests) == 1 && len(metas) == 1 {
		return &ExecuteResult{Output: out.String(), Metadata: metas[0]}, nil
	}
	return &ExecuteResult{Output: out.String(), Metadata: metas}, nil
}

func parseFileWriteRequests(params map[string]any) ([]fileWriteRequest, error) {
	var requests []fileWriteRequest
	if ok, err := decodeParamInto(params, "writes", &requests); err != nil {
		return nil, fmt.Errorf("invalid writes: %w", err)
	} else if ok {
		if len(requests) == 0 {
			return nil, fmt.Errorf("writes must contain at least one item")
		}
		for i := range requests {
			requests[i].FilePath = strings.TrimSpace(requests[i].FilePath)
			if requests[i].FilePath == "" {
				return nil, fmt.Errorf("writes[%d].file_path is required", i)
			}
		}
		return requests, nil
	}
	filePath := paramStringTrim(params, "file_path")
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required when writes is not provided")
	}
	return []fileWriteRequest{{FilePath: filePath, Content: paramString(params, "content")}}, nil
}

func (f *fileWriteTool) writeOne(ctx context.Context, req fileWriteRequest) (FileToolMetadata, string, error) {
	path, err := resolveFilePath(req.FilePath)
	if err != nil {
		return FileToolMetadata{}, "", err
	}
	content, err := validateTextBytes([]byte(req.Content))
	if err != nil {
		return FileToolMetadata{}, "", err
	}
	if len([]byte(content)) > fileWriteMaxSizeBytes {
		return FileToolMetadata{}, "", fmt.Errorf("content is too large to write (%d bytes, max %d)", len([]byte(content)), fileWriteMaxSizeBytes)
	}

	info, statErr := os.Stat(path)
	existed := false
	if statErr != nil && !os.IsNotExist(statErr) {
		return FileToolMetadata{}, "", fmt.Errorf("failed to stat file: %w", statErr)
	}
	var original string
	if statErr == nil {
		if info.IsDir() {
			return FileToolMetadata{}, "", fmt.Errorf("file_path is a directory, not a file: %s", path)
		}
		existed = true
		if info.Size() > fileWriteMaxSizeBytes {
			return FileToolMetadata{}, "", fmt.Errorf("file is too large to overwrite (%d bytes, max %d)", info.Size(), fileWriteMaxSizeBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return FileToolMetadata{}, "", fmt.Errorf("failed to read existing file: %w", err)
		}
		original, err = validateTextBytes(data)
		if err != nil {
			return FileToolMetadata{}, "", err
		}
		if err := requireFreshFullRead(ctx, path, original, info); err != nil {
			return FileToolMetadata{}, "", err
		}
		content = normalizeReplacementLineEndings(original, content)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return FileToolMetadata{}, "", fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return FileToolMetadata{}, "", fmt.Errorf("failed to write file: %w", err)
	}
	newInfo, _ := os.Stat(path)
	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{Content: content, MTimeUnix: fileMTimeUnix(newInfo), Offset: 1, Limit: 0, Partial: false})
	}
	if existed {
		return buildFileToolMetadata(path, "Write", fileToolSummary("Write", path), original, content),
			fmt.Sprintf("File updated successfully: %s\nBytes written: %d", path, len([]byte(content))), nil
	}
	return buildFileToolMetadata(path, "Create", fileToolSummary("Create", path), "", content),
		fmt.Sprintf("File created successfully: %s\nBytes written: %d", path, len([]byte(content))), nil
}
