package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type fileReadTool struct{}

type fileReadRange struct {
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

type fileReadRequest struct {
	FilePath string          `json:"file_path"`
	Ranges   []fileReadRange `json:"ranges,omitempty"`
}

func init() {
	Register(&fileReadTool{})
}

func (f *fileReadTool) Name() string { return "file_read" }

func (f *fileReadTool) Description() string {
	return "Read UTF-8 text files from the local filesystem with line numbers and optional ranges."
}

func (f *fileReadTool) Commands() []Command {
	return []Command{{
		Name:        "read",
		Description: fmt.Sprintf("Read UTF-8 text files. Supports single-file mode (file_path/offset/limit) and batch mode via requests[]. Default max %d lines per range.", fileReadDefaultMaxLines),
		Params: []CommandParam{
			{Name: "file_path", Required: false, Description: "Single-file mode path.", Example: "/path/to/file.go"},
			{Name: "offset", Required: false, Description: "Single-file mode 1-based start line.", Example: "120"},
			{Name: "limit", Required: false, Description: "Single-file mode max lines.", Example: "80"},
			{Name: "requests", Required: false, Description: "Batch mode: [{file_path,ranges:[{offset,limit}]}].", Example: `[{"file_path":"a.txt","ranges":[{"offset":1,"limit":3},{"offset":10,"limit":5}]}]`, Schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"file_path"},
				},
			}},
		},
	}}
}

func (f *fileReadTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "read":
		return f.read(ctx, params)
	default:
		return nil, fmt.Errorf("file_read tool does not support command: %s", command)
	}
}

func (f *fileReadTool) read(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	requests, err := parseFileReadRequests(params)
	if err != nil {
		return nil, err
	}

	var out strings.Builder
	success := 0
	failed := 0
	for i, req := range requests {
		if i > 0 {
			out.WriteString("\n\n")
		}
		block, blockErr := f.readOne(ctx, req)
		if blockErr != nil {
			if len(requests) == 1 {
				return nil, blockErr
			}
			failed++
			out.WriteString(fmt.Sprintf("Request %d failed: %v", i+1, blockErr))
			continue
		}
		success++
		out.WriteString(block)
	}

	out.WriteString(fmt.Sprintf("\n\nSummary: succeeded=%d failed=%d total=%d", success, failed, len(requests)))
	if len(requests) == 1 {
		return &ExecuteResult{Output: strings.TrimSpace(out.String())}, nil
	}
	return &ExecuteResult{Output: strings.TrimSpace(out.String())}, nil
}

func parseFileReadRequests(params map[string]any) ([]fileReadRequest, error) {
	var requests []fileReadRequest
	if ok, err := decodeParamInto(params, "requests", &requests); err != nil {
		return nil, fmt.Errorf("invalid requests: %w", err)
	} else if ok {
		if len(requests) == 0 {
			return nil, fmt.Errorf("requests must contain at least one item")
		}
		for i := range requests {
			requests[i].FilePath = strings.TrimSpace(requests[i].FilePath)
			if requests[i].FilePath == "" {
				return nil, fmt.Errorf("requests[%d].file_path is required", i)
			}
		}
		return requests, nil
	}

	filePath := paramStringTrim(params, "file_path")
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required when requests is not provided")
	}
	offset, _, err := paramInt(params, "offset")
	if err != nil {
		return nil, err
	}
	limit, _, err := paramInt(params, "limit")
	if err != nil {
		return nil, err
	}
	return []fileReadRequest{{
		FilePath: filePath,
		Ranges:   []fileReadRange{{Offset: offset, Limit: limit}},
	}}, nil
}

func (f *fileReadTool) readOne(ctx context.Context, req fileReadRequest) (string, error) {
	path, err := resolveFilePath(req.FilePath)
	if err != nil {
		return "", err
	}
	if isBlockedDevicePath(path) {
		return "", fmt.Errorf("cannot read %q: this device file would block or produce infinite output", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file does not exist: %s", path)
		}
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("file_path is a directory, not a file: %s", path)
	}
	if info.Size() > fileReadMaxSizeBytes {
		return "", fmt.Errorf("file is too large to read (%d bytes, max %d)", info.Size(), fileReadMaxSizeBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	content, err := validateTextBytes(data)
	if err != nil {
		return "", err
	}
	lines := splitTextLines(content)
	totalLines := len(lines)

	ranges := req.Ranges
	if len(ranges) == 0 {
		ranges = []fileReadRange{{Offset: 1, Limit: fileReadDefaultMaxLines}}
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("File: %s\nTotal lines: %d\n", path, totalLines))

	partial := false
	for idx, r := range ranges {
		offset := r.Offset
		if offset <= 0 {
			offset = 1
		}
		limit := r.Limit
		if limit <= 0 || limit > fileReadDefaultMaxLines {
			limit = fileReadDefaultMaxLines
		}

		start := offset - 1
		if start > totalLines {
			start = totalLines
		}
		end := start + limit
		truncated := false
		if end < totalLines {
			truncated = true
		} else {
			end = totalLines
		}
		if start > 0 {
			truncated = true
		}
		if truncated {
			partial = true
		}

		out.WriteString(fmt.Sprintf("Range %d lines %d-%d:\n", idx+1, start+1, end))
		for i := start; i < end; i++ {
			out.WriteString(fmt.Sprintf("%6d\t%s\n", i+1, lines[i]))
		}
		if truncated {
			out.WriteString("... [truncated; use offset/limit to read another range] ...\n")
		}
	}

	if len(ranges) > 1 {
		partial = true
	}
	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{
			Content:   content,
			MTimeUnix: fileMTimeUnix(info),
			Offset:    1,
			Limit:     0,
			Partial:   partial,
		})
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
