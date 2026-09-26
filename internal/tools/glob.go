package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"slimebot/internal/constants"
	sandboxpolicy "slimebot/internal/sandbox"
)

const (
	globDefaultLimit = 100
	globMaxLimit     = 1000
)

type globTool struct{}

func init() {
	Register(&globTool{})
}

func (g *globTool) Name() string { return "glob" }

func (g *globTool) Description() string {
	return "Find local files by filename or glob pattern using ripgrep, sorted by modification time."
}

func (g *globTool) Commands() []Command {
	return []Command{{
		Name:        "find",
		Description: "Find files matching a filename or glob pattern. Use this for filename searches; set path to ~/Downloads when the user mentions downloads/下载目录. Returns matching paths sorted by modification time.",
		Params: []CommandParam{
			{Name: "pattern", Required: true, Description: "Glob pattern to match files against.", Example: "**/*.go"},
			{Name: "path", Required: false, Description: "Directory to search. Defaults to current working directory.", Example: "/path/to/repo"},
			{Name: "limit", Required: false, Description: "Maximum files to return. Default 100, max 1000.", Example: "100"},
			{Name: "offset", Required: false, Description: "Skip the first N files before applying limit.", Example: "0"},
		},
	}}
}

func (g *globTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "find":
		return g.find(ctx, params)
	default:
		return nil, fmt.Errorf("glob tool does not support command: %s", command)
	}
}

func (g *globTool) find(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	pattern := paramStringTrim(params, "pattern")
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	rootRaw := paramStringTrim(params, "path")
	if rootRaw == "" && constants.ClientSurfaceFromContext(ctx) == constants.ClientSurfaceWeb && sandboxpolicy.WorkingDirectoryFromContext(ctx) == "" {
		return nil, fmt.Errorf("path is required when running from the web server because there is no user working directory")
	}
	if rootRaw == "" {
		rootRaw = "."
	}
	root, err := resolveFilePath(rootRaw, ctx)
	if err != nil {
		return nil, err
	}
	if err := checkSandboxRead(ctx, root); err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("failed to stat glob path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", root)
	}

	limit, _, err := paramInt(params, "limit")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = globDefaultLimit
	}
	if limit > globMaxLimit {
		limit = globMaxLimit
	}
	offset, _, err := paramInt(params, "offset")
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	searchDir, searchPattern := root, pattern
	if filepath.IsAbs(pattern) {
		base, rel := extractGlobBaseDirectory(pattern)
		if base != "" {
			searchDir = base
			searchPattern = rel
			if err := checkSandboxRead(ctx, searchDir); err != nil {
				return nil, err
			}
		}
	}

	args := []string{"--files", "--glob", searchPattern, "--sort=modified", "--hidden", "--no-ignore", "--no-messages"}
	rg, err := runRipgrepWithOptions(ctx, args, searchDir, ripgrepRunOptions{allowPartialOutput: true})
	if err != nil {
		return nil, err
	}
	total := len(rg.lines)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	limited := rg.lines[offset:end]
	truncated := rg.truncated || total > end
	if len(limited) == 0 {
		return &ExecuteResult{Output: "No files found."}, nil
	}
	paths := make([]string, 0, len(limited))
	for _, line := range limited {
		path := line
		if !filepath.IsAbs(path) {
			path = filepath.Join(searchDir, path)
		}
		paths = append(paths, displayPath(root, path))
	}
	var out strings.Builder
	out.WriteString(fmt.Sprintf("Found %d of %d files, truncated=%t", len(paths), total, truncated))
	if offset > 0 {
		out.WriteString(fmt.Sprintf(", offset=%d", offset))
	}
	out.WriteString("\n")
	out.WriteString(strings.Join(paths, "\n"))
	return &ExecuteResult{Output: out.String()}, nil
}

func extractGlobBaseDirectory(pattern string) (baseDir, relativePattern string) {
	first := strings.IndexAny(pattern, "*?[{")
	if first < 0 {
		return filepath.Dir(pattern), filepath.Base(pattern)
	}
	staticPrefix := pattern[:first]
	lastSep := strings.LastIndexAny(staticPrefix, `/\`)
	if lastSep < 0 {
		return "", pattern
	}
	baseDir = staticPrefix[:lastSep]
	relativePattern = pattern[lastSep+1:]
	if baseDir == "" && lastSep == 0 {
		baseDir = string(filepath.Separator)
	}
	return baseDir, relativePattern
}
