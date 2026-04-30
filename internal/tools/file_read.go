package tools

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type fileReadTool struct{}

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
		Description: fmt.Sprintf("Read a UTF-8 text file. Returns cat -n style line numbers. By default reads at most %d lines; use offset and limit for large files. This tool reads files only, not directories.", fileReadDefaultMaxLines),
		Params: []CommandParam{
			{Name: "file_path", Required: true, Description: "Absolute or relative path to the file to read.", Example: "/path/to/file.go"},
			{Name: "offset", Required: false, Description: "1-based line number to start reading from.", Example: "120"},
			{Name: "limit", Required: false, Description: "Maximum number of lines to read.", Example: "80"},
		},
	}}
}

func (f *fileReadTool) Execute(ctx context.Context, command string, params map[string]string) (*ExecuteResult, error) {
	switch command {
	case "read":
		return f.read(ctx, params)
	default:
		return nil, fmt.Errorf("file_read tool does not support command: %s", command)
	}
}

func (f *fileReadTool) read(ctx context.Context, params map[string]string) (*ExecuteResult, error) {
	path, err := resolveFilePath(params["file_path"])
	if err != nil {
		return nil, err
	}
	if isBlockedDevicePath(path) {
		return nil, fmt.Errorf("cannot read %q: this device file would block or produce infinite output", path)
	}

	offset, offsetSet, err := parsePositiveIntParam(params["offset"], "offset")
	if err != nil {
		return nil, err
	}
	if !offsetSet {
		offset = 1
	}
	limit, limitSet, err := parsePositiveIntParam(params["limit"], "limit")
	if err != nil {
		return nil, err
	}
	if !limitSet || limit > fileReadDefaultMaxLines {
		limit = fileReadDefaultMaxLines
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", path)
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("file_path is a directory, not a file: %s", path)
	}
	if info.Size() > fileReadMaxSizeBytes {
		return nil, fmt.Errorf("file is too large to read (%d bytes, max %d). Use a search or split the file into smaller ranges after narrowing the target", info.Size(), fileReadMaxSizeBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	content, err := validateTextBytes(data)
	if err != nil {
		return nil, err
	}

	lines := splitTextLines(content)
	totalLines := len(lines)
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
	if offsetSet || limitSet {
		truncated = truncated || start > 0
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("File: %s\n", path))
	out.WriteString(fmt.Sprintf("Total lines: %d\n", totalLines))
	if content == "" {
		out.WriteString("Warning: the file exists but is empty.\n")
	} else if start == totalLines {
		out.WriteString(fmt.Sprintf("Warning: offset %d is beyond the end of the file.\n", offset))
	} else {
		out.WriteString(fmt.Sprintf("Showing lines %d-%d:\n", start+1, end))
		for i := start; i < end; i++ {
			out.WriteString(fmt.Sprintf("%6d\t%s\n", i+1, lines[i]))
		}
	}
	if truncated {
		out.WriteString(fmt.Sprintf("... [truncated; use offset/limit to read another range] ...\n"))
	}

	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{
			Content:   content,
			MTimeUnix: fileMTimeUnix(info),
			Offset:    offset,
			Limit:     limit,
			Partial:   truncated,
		})
	}
	return &ExecuteResult{Output: strings.TrimRight(out.String(), "\n")}, nil
}

func parsePositiveIntParam(raw string, name string) (int, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, false, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, true, nil
}
