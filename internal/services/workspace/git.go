package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxGitChanges = 500
const gitTimeout = 3 * time.Second

var ErrGitTimeout = errors.New("workspace Git operation timed out")
var ErrNotRepository = errors.New("workspace is not a Git repository")

type GitChange struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath,omitempty"`
	Staged    string `json:"staged,omitempty"`
	Unstaged  string `json:"unstaged,omitempty"`
	Untracked bool   `json:"untracked,omitempty"`
}

type GitStatus struct {
	IsRepo    bool        `json:"isRepo"`
	Branch    string      `json:"branch,omitempty"`
	Upstream  string      `json:"upstream,omitempty"`
	Ahead     int         `json:"ahead"`
	Behind    int         `json:"behind"`
	Changes   []GitChange `json:"changes"`
	Truncated bool        `json:"truncated"`
}

type GitDiff struct {
	Path      string `json:"path"`
	Diff      string `json:"diff,omitempty"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	remaining := max(0, w.limit-w.buffer.Len())
	if len(p) > remaining {
		w.truncated = true
	}
	_, _ = w.buffer.Write(p[:min(remaining, len(p))])
	return len(p), nil
}

func runGit(ctx context.Context, dir string, limit int, args ...string) (string, bool, error) {
	args = append([]string{"--no-pager", "--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.WaitDelay = 100 * time.Millisecond
	for _, item := range cmd.Environ() {
		key := strings.ToUpper(strings.SplitN(item, "=", 2)[0])
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_EXTERNAL_DIFF", "GIT_PREFIX", "GIT_OPTIONAL_LOCKS", "LC_ALL":
			continue
		}
		cmd.Env = append(cmd.Env, item)
	}
	cmd.Env = append(cmd.Env, "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	stdout, stderr := &limitedBuffer{limit: limit}, &limitedBuffer{limit: 4096}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", false, ErrGitTimeout
	}
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || strings.Contains(stderr.buffer.String(), "not a git repository") {
			return "", false, ErrNotRepository
		}
		return "", false, fmt.Errorf("workspace Git command failed: %w: %s", err, strings.TrimSpace(stderr.buffer.String()))
	}
	return stdout.buffer.String(), stdout.truncated, nil
}

func Status(ctx context.Context, directory string) (GitStatus, error) {
	root, err := canonicalRoot(directory)
	if err != nil {
		return GitStatus{}, err
	}
	result := GitStatus{Changes: []GitChange{}}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	prefix, _, err := runGit(ctx, root, 8192, "rev-parse", "--show-prefix")
	if errors.Is(err, ErrNotRepository) {
		return result, nil
	}
	if err != nil {
		return GitStatus{}, err
	}
	raw, truncated, err := runGit(ctx, root, 4<<20, "status", "--porcelain=v2", "--branch", "-z", "-uall", "--", ".")
	if err != nil {
		return GitStatus{}, err
	}
	if truncated {
		if end := strings.LastIndexByte(raw, 0); end >= 0 {
			raw = raw[:end+1]
		}
	}
	result = parseStatus(raw, strings.TrimSuffix(prefix, "\n"))
	result.Truncated = result.Truncated || truncated
	return result, nil
}

func parseStatus(raw, prefix string) GitStatus {
	result := GitStatus{IsRepo: true, Changes: make([]GitChange, 0)}
	records := strings.Split(raw, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		if strings.HasPrefix(record, "# ") {
			header, value, _ := strings.Cut(record[2:], " ")
			switch header {
			case "branch.head":
				result.Branch = value
			case "branch.upstream":
				result.Upstream = value
			case "branch.ab":
				ahead, behind, _ := strings.Cut(value, " ")
				result.Ahead, _ = strconv.Atoi(strings.TrimPrefix(ahead, "+"))
				result.Behind, _ = strconv.Atoi(strings.TrimPrefix(behind, "-"))
			}
			continue
		}
		change := GitChange{}
		switch record[0] {
		case '?':
			change.Untracked = true
			change.Path = strings.TrimPrefix(record, "? ")
		case '1', '2', 'u':
			count := 9
			if record[0] == '2' {
				count = 10
			}
			if record[0] == 'u' {
				count = 11
			}
			fields := strings.SplitN(record, " ", count)
			if len(fields) != count || len(fields[1]) != 2 {
				continue
			}
			change.Path = fields[count-1]
			change.Staged = strings.Trim(fields[1][:1], ".")
			change.Unstaged = strings.Trim(fields[1][1:], ".")
			if record[0] == '2' && i+1 < len(records) {
				i++
				if strings.HasPrefix(records[i], prefix) {
					change.OldPath = strings.TrimPrefix(records[i], prefix)
				}
			}
		default:
			continue
		}
		if !strings.HasPrefix(change.Path, prefix) {
			continue
		}
		change.Path = strings.TrimPrefix(change.Path, prefix)
		if change.Path == "" || !validRelativePath(change.Path) {
			continue
		}
		if len(result.Changes) == maxGitChanges {
			result.Truncated = true
			break
		}
		result.Changes = append(result.Changes, change)
	}
	if result.Branch == "(detached)" {
		result.Branch = "HEAD"
	}
	return result
}

func Diff(ctx context.Context, directory, path string, staged bool) (GitDiff, error) {
	root, err := canonicalRoot(directory)
	if err != nil {
		return GitDiff{}, err
	}
	if path == "" || path == "." {
		return GitDiff{}, ErrInvalidPath
	}
	if _, err := resolveSafePath(root, path, true); err != nil {
		return GitDiff{}, err
	}
	path = filepath.ToSlash(filepath.Clean(path))
	result := GitDiff{Path: path}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	args := []string{"diff", "--relative", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames"}
	if staged {
		args = append(args, "--cached")
	}
	raw, truncated, err := runGit(ctx, root, MaxPreviewBytes, append(args, "--", path)...)
	if errors.Is(err, ErrNotRepository) {
		return result, nil
	}
	if err != nil {
		return GitDiff{}, err
	}
	if !staged && raw == "" {
		untracked, _, err := runGit(ctx, root, 8192, "ls-files", "--others", "--exclude-standard", "-z", "--", path)
		if err != nil {
			return GitDiff{}, err
		}
		if untracked == path+"\x00" {
			return untrackedDiff(root, path)
		}
	}
	result.Binary = strings.ContainsRune(raw, 0) || strings.Contains(raw, "\nBinary files ") || strings.Contains(raw, "\nGIT binary patch\n")
	result.Truncated = truncated
	if !result.Binary {
		result.Diff = raw
	}
	return result, nil
}

func untrackedDiff(root, path string) (GitDiff, error) {
	file, err := ReadFile(root, path)
	if err != nil {
		return GitDiff{}, err
	}
	result := GitDiff{Path: path, Binary: file.Binary, Truncated: file.Truncated}
	if result.Binary || result.Truncated {
		return result, nil
	}
	name := "b/" + path
	old := "a/" + path
	if strings.ContainsAny(path, "\t\r\n\"") {
		name, old = strconv.Quote(name), strconv.Quote(old)
	}
	prefix := "diff --git " + old + " " + name + "\nnew file mode 100644\n"
	if file.Content == "" {
		result.Diff = prefix
		return result, nil
	}
	lines := strings.SplitAfter(file.Content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var diff strings.Builder
	diff.WriteString(prefix)
	fmt.Fprintf(&diff, "--- /dev/null\n+++ %s\n@@ -0,0 +1,%d @@\n", name, len(lines))
	for _, line := range lines {
		diff.WriteByte('+')
		diff.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			diff.WriteString("\n\\ No newline at end of file\n")
		}
	}
	raw := diff.String()
	result.Truncated = len(raw) > MaxPreviewBytes
	result.Diff = raw[:min(len(raw), MaxPreviewBytes)]
	return result, nil
}
