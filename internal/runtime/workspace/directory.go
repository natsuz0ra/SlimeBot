package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slimebot/internal/domain"

	"github.com/google/uuid"
)

func artifactGitEnv(a domain.AgentArtifact) []string {
	if a.GitDirectory == "" {
		return nil
	}
	return []string{"GIT_DIR=" + a.GitDirectory, "GIT_WORK_TREE=" + a.ParentWorkspace}
}

// createDirectoryWorkspace uses an external private Git database, keeping the
// user's directory free of .git files. The same snapshot, preview and recovery
// rules then apply to both repository and ordinary-directory workspaces.
// The caller holds the mutation lock.
func (m *Manager) createDirectoryWorkspace(ctx context.Context, parent string) (*domain.AgentArtifact, error) {
	root, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrInvalid
	}
	id := uuid.NewString()
	gitDirectory := filepath.Join(m.directory, "git", id+".git")
	if err = os.MkdirAll(filepath.Dir(gitDirectory), 0700); err != nil {
		return nil, err
	}
	if _, err = git(ctx, root, nil, "init", "--bare", gitDirectory); err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(gitDirectory)
		}
	}()
	a := &domain.AgentArtifact{ID: id, GitDirectory: gitDirectory, ParentWorkspace: root, Workspace: filepath.Join(m.directory, id), Status: "working"}
	env := artifactGitEnv(*a)
	base, err := snapshot(ctx, root, env...)
	if err != nil {
		return nil, err
	}
	if _, err = git(ctx, root, env, "update-ref", "refs/heads/slimebot-baseline", base); err != nil {
		return nil, err
	}
	if _, err = git(ctx, root, env, "symbolic-ref", "HEAD", "refs/heads/slimebot-baseline"); err != nil {
		return nil, err
	}
	if _, err = git(ctx, root, env, "update-ref", "refs/slimebot/artifacts/"+id+"/base", base); err != nil {
		return nil, err
	}
	if _, err = git(ctx, root, env, "worktree", "add", "--detach", a.Workspace, base); err != nil {
		return nil, err
	}
	a.BaseCommit = base
	keep = true
	return a, nil
}
