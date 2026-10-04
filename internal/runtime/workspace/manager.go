// Package workspace manages isolated Git worktrees and recoverable integration.
package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"slimebot/internal/domain"
	"strings"
	"sync"
)

type Manager struct {
	directory       string
	mu              *sync.Mutex
	CheckWrite      func(string) error
	ValidatePreview func(context.Context, string, string) error
}

var mutations sync.Mutex

func New(directory string) *Manager { return &Manager{directory: directory, mu: &mutations} }
func git(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	text, err := gitRaw(ctx, dir, env, args...)
	return strings.TrimSpace(text), err
}
func gitRaw(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	b, e := c.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("git %s failed: %s", args[0], strings.TrimSpace(string(b)))
	}
	return string(b), nil
}
func protected(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "config.cfg" || base == ".env" || strings.HasPrefix(base, ".env.") || base == "id_rsa" || base == "id_ed25519" || strings.HasSuffix(base, ".sqlite") || strings.HasSuffix(base, ".sqlite3") || strings.HasSuffix(base, ".db")
}
func snapshot(ctx context.Context, parent string, extraEnv ...string) (string, error) {
	f, e := os.CreateTemp("", "slimebot-index-*")
	if e != nil {
		return "", e
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	defer os.Remove(name)
	env := append(append([]string{}, extraEnv...), "GIT_INDEX_FILE="+name, "GIT_AUTHOR_NAME=SlimeBot", "GIT_AUTHOR_EMAIL=slimebot@localhost", "GIT_COMMITTER_NAME=SlimeBot", "GIT_COMMITTER_EMAIL=slimebot@localhost")
	head, headErr := git(ctx, parent, extraEnv, "rev-parse", "--verify", "HEAD")
	if headErr == nil {
		_, e = git(ctx, parent, env, "read-tree", "HEAD")
	} else {
		_, e = git(ctx, parent, env, "read-tree", "--empty")
	}
	if e != nil {
		return "", e
	}
	// Enumerate from the temporary baseline index so staged deletions are included.
	files, e := gitRaw(ctx, parent, env, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if e != nil {
		return "", e
	}
	for _, path := range strings.Split(files, "\x00") {
		if path == "" {
			continue
		}
		if protected(path) {
			return "", fmt.Errorf("protected configuration cannot be copied into agent worktree: %s", path)
		}
		if _, e = git(ctx, parent, env, "add", "--", path); e != nil {
			return "", e
		}
	}
	tree, e := git(ctx, parent, env, "write-tree")
	if e != nil {
		return "", e
	}
	args := []string{"commit-tree", tree}
	if headErr == nil {
		args = append(args, "-p", head)
	}
	return git(ctx, parent, env, append(args, "-m", "SlimeBot local workspace snapshot")...)
}
func (m *Manager) Create(ctx context.Context, parent string) (*domain.AgentArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, e := git(ctx, parent, nil, "rev-parse", "--show-toplevel")
	if e != nil {
		// A directory without Git still gets an isolated, recoverable workspace.
		// Its private Git database is held under the managed runtime directory.
		return m.createDirectoryWorkspace(ctx, parent)
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(m.directory, 0700); e != nil {
		return nil, e
	}
	base, e := snapshot(ctx, root)
	if e != nil {
		return nil, e
	}
	id := uuid.NewString()
	path := filepath.Join(m.directory, id)
	if _, e = git(ctx, root, nil, "update-ref", "refs/slimebot/artifacts/"+id+"/base", base); e != nil {
		return nil, e
	}
	if _, e = git(ctx, root, nil, "worktree", "add", "--detach", path, base); e != nil {
		_, _ = git(context.WithoutCancel(ctx), root, nil, "update-ref", "-d", "refs/slimebot/artifacts/"+id+"/base")
		return nil, e
	}
	return &domain.AgentArtifact{ID: id, ParentWorkspace: root, Workspace: path, BaseCommit: base, Status: "working"}, nil
}
func (m *Manager) Capture(ctx context.Context, a *domain.AgentArtifact) error {
	commit, e := snapshot(ctx, a.Workspace)
	if e != nil {
		return e
	}
	if a.ResultCommit != "" {
		oldTree, err := git(ctx, a.Workspace, nil, "rev-parse", a.ResultCommit+"^{tree}")
		if err != nil {
			return err
		}
		newTree, err := git(ctx, a.Workspace, nil, "rev-parse", commit+"^{tree}")
		if err != nil {
			return err
		}
		if oldTree == newTree {
			commit = a.ResultCommit
		}
	}
	if a.ResultCommit != commit {
		a.ValidatedCommit = ""
		a.ValidationCommand = ""
	}
	if _, e = git(ctx, a.Workspace, nil, "update-ref", "refs/slimebot/artifacts/"+a.ID+"/result", commit); e != nil {
		return e
	}
	a.ResultCommit = commit
	a.Status = "ready"
	return nil
}

// MatchesResult detects edits after capture, including edits made by a validation command.
func (m *Manager) MatchesResult(ctx context.Context, a domain.AgentArtifact) (bool, error) {
	return matchesTree(ctx, a.Workspace, a.ResultCommit)
}
func matchesTree(ctx context.Context, directory, commit string) (bool, error) {
	current, err := snapshot(ctx, directory)
	if err != nil {
		return false, err
	}
	actual, err := git(ctx, directory, nil, "rev-parse", current+"^{tree}")
	if err != nil {
		return false, err
	}
	expected, err := git(ctx, directory, nil, "rev-parse", commit+"^{tree}")
	return actual == expected, err
}
func (m *Manager) Diff(ctx context.Context, a domain.AgentArtifact) (string, error) {
	if a.ResultCommit == "" {
		return "", errors.New("artifact is not ready")
	}
	text, e := gitRaw(ctx, a.ParentWorkspace, artifactGitEnv(a), "diff", "--no-ext-diff", a.BaseCommit, a.ResultCommit, "--")
	if len(text) > 256*1024 {
		text = text[:256*1024] + "\n[diff truncated]"
	}
	return text, e
}
func contentHash(dir string, paths []string) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		b, e := os.ReadFile(filepath.Join(dir, p))
		if os.IsNotExist(e) {
			h.Write([]byte("missing"))
			continue
		}
		if e != nil {
			return "", e
		}
		info, e := os.Lstat(filepath.Join(dir, p))
		if e != nil {
			return "", e
		}
		fmt.Fprintf(h, "%d:", info.Mode().Perm())
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (m *Manager) Integrate(ctx context.Context, a *domain.AgentArtifact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, err := os.ReadFile(m.journalPath(a.ID)); err == nil {
		var j journal
		if err = json.Unmarshal(data, &j); err != nil {
			return err
		}
		if j.Completed {
			*a = j.Artifact
			return nil
		}
		if err = restoreJournal(j); err != nil {
			return err
		}
		if err = os.Remove(m.journalPath(a.ID)); err != nil {
			return err
		}
	}
	if a.Status != "ready" && a.Status != "conflict" && a.Status != "validation_failed" {
		return errors.New("artifact is not ready for integration")
	}
	if a.ResultCommit == "" {
		return errors.New("artifact has no captured result")
	}
	raw, e := gitRaw(ctx, a.ParentWorkspace, artifactGitEnv(*a), "diff", "--no-renames", "--name-only", "-z", a.BaseCommit, a.ResultCommit)
	if e != nil {
		return e
	}
	var paths []string
	for _, p := range strings.Split(raw, "\x00") {
		if p != "" {
			if filepath.IsAbs(p) || strings.HasPrefix(filepath.Clean(p), "..") {
				return errors.New("invalid patch path")
			}
			if protected(p) {
				return errors.New("protected artifact path")
			}
			if err := safePath(a.ParentWorkspace, p); err != nil {
				return err
			}
			if err := checkScope(a.WriteScopes, p); err != nil {
				return err
			}
			if m.CheckWrite != nil {
				if err := m.CheckWrite(filepath.Join(a.ParentWorkspace, p)); err != nil {
					return err
				}
			}
			paths = append(paths, p)
		}
	}
	before, e := contentHash(a.ParentWorkspace, paths)
	if e != nil {
		return e
	}
	parent, e := snapshot(ctx, a.ParentWorkspace, artifactGitEnv(*a)...)
	if e != nil {
		return e
	}
	preview := filepath.Join(m.directory, uuid.NewString())
	if _, e = git(ctx, a.ParentWorkspace, artifactGitEnv(*a), "worktree", "add", "--detach", preview, parent); e != nil {
		return e
	}
	defer git(context.WithoutCancel(ctx), a.ParentWorkspace, artifactGitEnv(*a), "worktree", "remove", "--force", preview)
	patch, e := gitRaw(ctx, a.ParentWorkspace, artifactGitEnv(*a), "diff", "--no-renames", "--binary", a.BaseCommit, a.ResultCommit)
	if e != nil {
		return e
	}
	c := exec.CommandContext(ctx, "git", "apply", "--3way")
	c.Dir = preview
	c.Stdin = strings.NewReader(patch)
	if out, err := c.CombinedOutput(); err != nil && len(paths) > 0 {
		a.Status = "conflict"
		a.Report = string(out)
		return errors.New("INTEGRATION_CONFLICT")
	}
	if m.ValidatePreview != nil {
		merged, err := snapshot(ctx, preview)
		if err != nil {
			return err
		}
		if err = m.ValidatePreview(ctx, preview, a.ValidationCommand); err != nil {
			a.Status = "validation_failed"
			a.Report = "Merged workspace validation failed: " + err.Error()
			return err
		}
		matches, err := matchesTree(ctx, preview, merged)
		if err != nil {
			return err
		}
		if !matches {
			return errors.New("validation modified the merged source; parent left unchanged")
		}
	}
	after, e := contentHash(a.ParentWorkspace, paths)
	if e != nil {
		return e
	}
	if before != after {
		return domain.ErrAgentConflict
	}
	backup := map[string][]byte{}
	modes := map[string]os.FileMode{}
	missing := map[string]bool{}
	for _, p := range paths {
		full := filepath.Join(a.ParentWorkspace, p)
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			missing[p] = true
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("integration of symbolic links requires manual review")
		}
		b, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		backup[p] = b
		modes[p] = info.Mode()
	}
	rollback := func() {
		for _, p := range paths {
			full := filepath.Join(a.ParentWorkspace, p)
			if missing[p] {
				_ = os.Remove(full)
			} else {
				_ = os.WriteFile(full, backup[p], modes[p])
				_ = os.Chmod(full, modes[p])
			}
		}
	}
	j := journal{Artifact: *a}
	for _, p := range paths {
		src := filepath.Join(preview, p)
		info, err := os.Lstat(src)
		deleted := os.IsNotExist(err)
		if err != nil && !deleted {
			return err
		}
		if !deleted && !info.Mode().IsRegular() {
			return errors.New("nonregular artifact file")
		}
		var after []byte
		if !deleted {
			after, err = os.ReadFile(src)
			if err != nil {
				return err
			}
		}
		j.Files = append(j.Files, journalFile{Path: p, Before: backup[p], After: after, Missing: missing[p], Deleted: deleted, Mode: modes[p]})
	}
	if e = writeJournal(m.journalPath(a.ID), j); e != nil {
		return e
	}
	for _, p := range paths {
		src := filepath.Join(preview, p)
		dst := filepath.Join(a.ParentWorkspace, p)
		info, err := os.Lstat(src)
		if os.IsNotExist(err) {
			if err = os.Remove(dst); err != nil && !os.IsNotExist(err) {
				rollback()
				return err
			}
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			rollback()
			return errors.New("nonregular artifact file")
		}
		b, err := os.ReadFile(src)
		if err != nil {
			rollback()
			return err
		}
		if bytes.Equal(b, backup[p]) && !missing[p] && info.Mode().Perm() == modes[p].Perm() {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			rollback()
			return err
		}
		if err = os.WriteFile(dst, b, info.Mode()); err != nil {
			rollback()
			return err
		}
		if err = os.Chmod(dst, info.Mode()); err != nil {
			rollback()
			return err
		}
	}
	a.Status = "integrated"
	a.Report = "Three-way integration completed; parent index preserved."
	j.Completed = true
	j.Artifact = *a
	if e = writeJournal(m.journalPath(a.ID), j); e != nil {
		rollback()
		return e
	}
	return nil
}

// Reject links in every existing path component before reading or writing. This
// also covers absent leaves reached through a symlinked parent directory.
func safePath(root, path string) error {
	current := root
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == ".." || part == "." || part == "" {
			return errors.New("invalid artifact path")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symbolic artifact paths require manual integration")
		}
	}
	return nil
}
