package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"slimebot/internal/constants"
)

const (
	grepDefaultHeadLimit = 250
	grepMaxHeadLimit     = 1000
)

var grepVCSDirectories = []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}

type grepTool struct{}

func init() {
	Register(&grepTool{})
}

func (g *grepTool) Name() string { return "grep" }

func (g *grepTool) Description() string {
	return "Search local file contents with ripgrep regex, glob/type filters, output modes, and pagination. Use glob to locate files by filename."
}

func (g *grepTool) Commands() []Command {
	return []Command{{
		Name:        "search",
		Description: "Search file contents using ripgrep. Defaults to returning files with matches; use glob__find instead when the user is looking for a filename.",
		Params: []CommandParam{
			{Name: "pattern", Required: true, Description: "Regular expression pattern to search for.", Example: "func\\s+BuildToolDefs"},
			{Name: "path", Required: false, Description: "File or directory to search. Defaults to current working directory.", Example: "/path/to/repo"},
			{Name: "glob", Required: false, Description: "Glob pattern(s) to filter files, split on spaces or commas except brace groups.", Example: "*.go"},
			{Name: "output_mode", Required: false, Description: "One of content, files_with_matches, or count. Defaults to files_with_matches.", Example: "content"},
			{Name: "type", Required: false, Description: "Ripgrep file type filter passed to --type.", Example: "go"},
			{Name: "case_insensitive", Required: false, Description: "Case insensitive search.", Example: "true", Schema: map[string]any{"type": "boolean"}},
			{Name: "context", Required: false, Description: "Lines before and after each match in content mode.", Example: "2"},
			{Name: "before", Required: false, Description: "Lines before each match in content mode.", Example: "2"},
			{Name: "after", Required: false, Description: "Lines after each match in content mode.", Example: "2"},
			{Name: "line_numbers", Required: false, Description: "Show line numbers in content mode. Defaults to true.", Example: "true", Schema: map[string]any{"type": "boolean"}},
			{Name: "head_limit", Required: false, Description: "Limit output entries after offset. Default 250, max 1000. Use 0 for unlimited.", Example: "100"},
			{Name: "offset", Required: false, Description: "Skip the first N output entries before applying head_limit.", Example: "0"},
			{Name: "multiline", Required: false, Description: "Enable multiline mode and dotall matching.", Example: "false", Schema: map[string]any{"type": "boolean"}},
		},
	}}
}

func (g *grepTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "search":
		return g.search(ctx, params)
	default:
		return nil, fmt.Errorf("grep tool does not support command: %s", command)
	}
}

func (g *grepTool) search(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	pattern := paramString(params, "pattern")
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	rootRaw := paramStringTrim(params, "path")
	if rootRaw == "" && constants.ClientSurfaceFromContext(ctx) == constants.ClientSurfaceWeb {
		return nil, fmt.Errorf("path is required when running from the web server because there is no user working directory")
	}
	if rootRaw == "" {
		rootRaw = "."
	}
	root, err := resolveFilePath(rootRaw)
	if err != nil {
		return nil, err
	}
	if err := checkSandboxRead(ctx, root); err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("failed to stat search path: %w", err)
	}

	outputMode := paramStringTrim(params, "output_mode")
	if outputMode == "" {
		outputMode = "files_with_matches"
	}
	if outputMode != "content" && outputMode != "files_with_matches" && outputMode != "count" {
		return nil, fmt.Errorf("output_mode must be content, files_with_matches, or count")
	}

	headLimit, _, err := paramInt(params, "head_limit")
	if err != nil {
		return nil, err
	}
	if headLimit < 0 {
		return nil, fmt.Errorf("head_limit must be >= 0")
	}
	if headLimit > grepMaxHeadLimit {
		headLimit = grepMaxHeadLimit
	}
	offset, _, err := paramInt(params, "offset")
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	args := []string{"--hidden"}
	for _, dir := range grepVCSDirectories {
		args = append(args, "--glob", "!"+dir)
	}
	args = append(args, "--max-columns", "500", "--with-filename")

	if multiline, _, err := paramBool(params, "multiline"); err != nil {
		return nil, err
	} else if multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if insensitive, _, err := paramBool(params, "case_insensitive"); err != nil {
		return nil, err
	} else if insensitive {
		args = append(args, "-i")
	}
	switch outputMode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	case "content":
		lineNumbers := true
		if value, ok, err := paramBool(params, "line_numbers"); err != nil {
			return nil, err
		} else if ok {
			lineNumbers = value
		}
		if lineNumbers {
			args = append(args, "-n")
		}
		if contextLines, ok, err := paramInt(params, "context"); err != nil {
			return nil, err
		} else if ok {
			args = append(args, "-C", strconv.Itoa(contextLines))
		} else {
			if before, ok, err := paramInt(params, "before"); err != nil {
				return nil, err
			} else if ok {
				args = append(args, "-B", strconv.Itoa(before))
			}
			if after, ok, err := paramInt(params, "after"); err != nil {
				return nil, err
			} else if ok {
				args = append(args, "-A", strconv.Itoa(after))
			}
		}
	}

	if strings.HasPrefix(pattern, "-") {
		args = append(args, "-e", pattern)
	} else {
		args = append(args, pattern)
	}
	if typ := paramStringTrim(params, "type"); typ != "" {
		args = append(args, "--type", typ)
	}
	for _, globPattern := range splitRipgrepGlobParam(paramStringTrim(params, "glob")) {
		args = append(args, "--glob", globPattern)
	}

	rg, err := runRipgrep(ctx, args, root)
	if err != nil {
		return nil, err
	}
	if outputMode == "files_with_matches" {
		sortRipgrepFilesByMTime(rg.lines)
	}
	limited, appliedLimit := applyRipgrepLimit(rg.lines, headLimit, offset)
	if outputMode == "content" {
		return &ExecuteResult{Output: formatGrepContentOutput(root, limited, appliedLimit, offset, rg.truncated)}, nil
	}
	if outputMode == "count" {
		return &ExecuteResult{Output: formatGrepCountOutput(root, limited, appliedLimit, offset, rg.truncated)}, nil
	}
	return &ExecuteResult{Output: formatGrepFilesOutput(root, limited, appliedLimit, offset, rg.truncated)}, nil
}

func splitRipgrepGlobParam(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, chunk := range strings.Fields(raw) {
		if strings.Contains(chunk, "{") && strings.Contains(chunk, "}") {
			out = append(out, chunk)
			continue
		}
		for _, part := range strings.Split(chunk, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func applyRipgrepLimit(lines []string, limit, offset int) ([]string, int) {
	if offset > len(lines) {
		return nil, 0
	}
	if limit == 0 {
		return lines[offset:], 0
	}
	if limit <= 0 {
		limit = grepDefaultHeadLimit
	}
	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}
	applied := 0
	if len(lines)-offset > limit {
		applied = limit
	}
	return lines[offset:end], applied
}

func sortRipgrepFilesByMTime(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		left, lerr := os.Stat(paths[i])
		right, rerr := os.Stat(paths[j])
		if lerr != nil || rerr != nil {
			return paths[i] < paths[j]
		}
		if left.ModTime().Equal(right.ModTime()) {
			return paths[i] < paths[j]
		}
		return left.ModTime().After(right.ModTime())
	})
}

func formatGrepContentOutput(root string, lines []string, appliedLimit, offset int, truncated bool) string {
	if len(lines) == 0 {
		return "No matches found."
	}
	formatted := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		formatted = append(formatted, formatRipgrepContentLine(root, line))
	}
	if hint := ripgrepPaginationHint(appliedLimit, offset, truncated); hint != "" {
		formatted = append(formatted, hint)
	}
	return strings.Join(formatted, "\n")
}

func formatGrepFilesOutput(root string, lines []string, appliedLimit, offset int, truncated bool) string {
	if len(lines) == 0 {
		return "No files found."
	}
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		paths = append(paths, displayPath(root, line))
	}
	header := fmt.Sprintf("Found %d %s", len(paths), pluralWord(len(paths), "file"))
	if hint := ripgrepInlineLimitInfo(appliedLimit, offset); hint != "" {
		header += " " + hint
	}
	if truncated {
		header += " truncated=true"
	}
	return header + "\n" + strings.Join(paths, "\n")
}

func formatGrepCountOutput(root string, lines []string, appliedLimit, offset int, truncated bool) string {
	if len(lines) == 0 {
		return "No matches found."
	}
	var total int
	var formatted []string
	for _, line := range lines {
		path, countText, ok := splitRipgrepPathLine(line)
		if !ok {
			formatted = append(formatted, line)
			continue
		}
		count, _ := strconv.Atoi(strings.TrimSpace(countText))
		total += count
		formatted = append(formatted, fmt.Sprintf("%s:%d", displayPath(root, path), count))
	}
	summary := fmt.Sprintf("Found %d total %s across %d %s", total, pluralWord(total, "occurrence"), len(lines), pluralWord(len(lines), "file"))
	if hint := ripgrepInlineLimitInfo(appliedLimit, offset); hint != "" {
		summary += " with pagination = " + hint
	}
	if truncated {
		summary += " truncated=true"
	}
	return strings.Join(formatted, "\n") + "\n\n" + summary
}

func formatRipgrepContentLine(root, line string) string {
	path, rest, ok := splitRipgrepPathLine(line)
	if !ok {
		return line
	}
	second := strings.Index(rest, ":")
	if second > 0 {
		lineNo := rest[:second]
		content := rest[second+1:]
		if _, err := strconv.Atoi(lineNo); err == nil {
			return fmt.Sprintf("%s L%s: %s", displayPath(root, path), lineNo, content)
		}
	}
	return fmt.Sprintf("%s: %s", displayPath(root, path), rest)
}

func splitRipgrepPathLine(line string) (string, string, bool) {
	start := len(filepath.VolumeName(line))
	separator := strings.IndexByte(line[start:], ':')
	if separator < 0 || start+separator == 0 {
		return "", "", false
	}
	separator += start
	return line[:separator], line[separator+1:], true
}

func displayPath(root, path string) string {
	clean := filepath.Clean(path)
	if rel, err := filepath.Rel(root, clean); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, clean); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(clean)
}

func ripgrepPaginationHint(appliedLimit, offset int, truncated bool) string {
	var parts []string
	if appliedLimit > 0 {
		parts = append(parts, fmt.Sprintf("limit: %d", appliedLimit))
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("offset: %d", offset))
	}
	if truncated {
		parts = append(parts, "truncated=true")
	}
	if len(parts) == 0 {
		return ""
	}
	return "[Showing results with pagination = " + strings.Join(parts, ", ") + "]"
}

func ripgrepInlineLimitInfo(appliedLimit, offset int) string {
	var parts []string
	if appliedLimit > 0 {
		parts = append(parts, fmt.Sprintf("limit: %d", appliedLimit))
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("offset: %d", offset))
	}
	return strings.Join(parts, ", ")
}

func pluralWord(n int, singular string) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}
