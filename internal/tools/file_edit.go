package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type fileEditTool struct{}

type fileEditOperation struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

type fileEditRequest struct {
	FilePath   string              `json:"file_path"`
	Operations []fileEditOperation `json:"operations"`
}

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
		Description: "Supports single operation and batch edits. Existing files must be fully read with file_read first.",
		Params: []CommandParam{
			{Name: "file_path", Required: false, Description: "Single-edit mode file path.", Example: "/path/to/file.go"},
			{Name: "old_string", Required: false, Description: "Single-edit mode old text.", Example: "old text"},
			{Name: "new_string", Required: false, Description: "Single-edit mode new text.", Example: "new text"},
			{Name: "replace_all", Required: false, Description: "Single-edit mode replace all matches.", Example: "false"},
			{Name: "edits", Required: false, Description: "Batch mode edits: [{file_path,operations:[{old_string,new_string,replace_all}]}].", Schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"file_path", "operations"},
				},
			}},
		},
	}}
}

func (f *fileEditTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "edit":
		return f.edit(ctx, params)
	default:
		return nil, fmt.Errorf("file_edit tool does not support command: %s", command)
	}
}

func (f *fileEditTool) edit(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	requests, err := parseFileEditRequests(params)
	if err != nil {
		return nil, err
	}

	var out strings.Builder
	success := 0
	failed := 0
	metas := make([]FileToolMetadata, 0, len(requests))
	for i, req := range requests {
		meta, msg, editErr := f.editOne(ctx, req)
		if i > 0 {
			out.WriteString("\n")
		}
		if editErr != nil {
			if len(requests) == 1 {
				return nil, editErr
			}
			failed++
			out.WriteString(fmt.Sprintf("Edit %d failed: %v", i+1, editErr))
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

func parseFileEditRequests(params map[string]any) ([]fileEditRequest, error) {
	var requests []fileEditRequest
	if ok, err := decodeParamInto(params, "edits", &requests); err != nil {
		return nil, fmt.Errorf("invalid edits: %w", err)
	} else if ok {
		if len(requests) == 0 {
			return nil, fmt.Errorf("edits must contain at least one item")
		}
		for i := range requests {
			requests[i].FilePath = strings.TrimSpace(requests[i].FilePath)
			if requests[i].FilePath == "" {
				return nil, fmt.Errorf("edits[%d].file_path is required", i)
			}
			if len(requests[i].Operations) == 0 {
				return nil, fmt.Errorf("edits[%d].operations must contain at least one operation", i)
			}
		}
		return requests, nil
	}

	replaceAll, _, err := paramBool(params, "replace_all")
	if err != nil {
		return nil, err
	}
	filePath := paramStringTrim(params, "file_path")
	oldString := paramString(params, "old_string")
	newString := paramString(params, "new_string")
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required when edits is not provided")
	}
	return []fileEditRequest{{
		FilePath: filePath,
		Operations: []fileEditOperation{{
			OldString:  oldString,
			NewString:  newString,
			ReplaceAll: replaceAll,
		}},
	}}, nil
}

func (f *fileEditTool) editOne(ctx context.Context, req fileEditRequest) (FileToolMetadata, string, error) {
	path, err := resolveFilePath(req.FilePath)
	if err != nil {
		return FileToolMetadata{}, "", err
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return FileToolMetadata{}, "", fmt.Errorf("failed to stat file: %w", statErr)
	}
	if statErr == nil && info.IsDir() {
		return FileToolMetadata{}, "", fmt.Errorf("file_path is a directory, not a file: %s", path)
	}

	var original string
	var existed bool
	if os.IsNotExist(statErr) {
		original = ""
	} else {
		existed = true
		if info.Size() > fileWriteMaxSizeBytes {
			return FileToolMetadata{}, "", fmt.Errorf("file is too large to edit (%d bytes, max %d)", info.Size(), fileWriteMaxSizeBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return FileToolMetadata{}, "", fmt.Errorf("failed to read file: %w", err)
		}
		original, err = validateTextBytes(data)
		if err != nil {
			return FileToolMetadata{}, "", err
		}
		if err := requireFreshFullRead(ctx, path, original, info); err != nil {
			return FileToolMetadata{}, "", err
		}
	}

	updated := original
	totalReplacements := 0
	for idx, op := range req.Operations {
		if op.OldString == op.NewString {
			return FileToolMetadata{}, "", fmt.Errorf("operation %d: old_string and new_string are exactly the same", idx+1)
		}
		actualNew := normalizeReplacementLineEndings(updated, op.NewString)
		next, count, applyErr := applyTextEdit(updated, op.OldString, actualNew, op.ReplaceAll)
		if applyErr != nil {
			return FileToolMetadata{}, "", fmt.Errorf("operation %d failed: %w", idx+1, applyErr)
		}
		totalReplacements += count
		updated = next
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return FileToolMetadata{}, "", fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return FileToolMetadata{}, "", fmt.Errorf("failed to write file: %w", err)
	}
	newInfo, _ := os.Stat(path)
	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{Content: updated, MTimeUnix: fileMTimeUnix(newInfo), Offset: 1, Limit: 0, Partial: false})
	}

	action := "Update"
	verb := "updated"
	if !existed {
		action = "Create"
		verb = "created"
	}
	meta := buildFileToolMetadata(path, action, fileToolSummary(action, path), original, updated)
	msg := fmt.Sprintf("File %s successfully: %s\nReplacements: %d", verb, path, totalReplacements)
	return meta, msg, nil
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
