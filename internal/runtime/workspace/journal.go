package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slimebot/internal/domain"
)

type journalFile struct {
	Path             string
	Before, After    []byte
	Missing, Deleted bool
	Mode             os.FileMode
}
type journal struct {
	Artifact  domain.AgentArtifact
	Completed bool
	Files     []journalFile
}

func (m *Manager) journalPath(id string) string {
	return filepath.Join(m.directory, "journals", id+".json")
}
func writeJournal(path string, j journal) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "journal-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func restoreJournal(j journal) error {
	// Validate the entire set before rollback, preserving edits made outside the
	// integration after a crash. Unknown state requires explicit manual review.
	for _, f := range j.Files {
		if err := safePath(j.Artifact.ParentWorkspace, f.Path); err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(j.Artifact.ParentWorkspace, f.Path))
		missing := os.IsNotExist(err)
		if err != nil && !missing {
			return err
		}
		original := (f.Missing && missing) || (!f.Missing && !missing && bytes.Equal(b, f.Before))
		desired := (f.Deleted && missing) || (!f.Deleted && !missing && bytes.Equal(b, f.After))
		if !original && !desired {
			return errors.New("integration recovery requires review: parent files changed")
		}
	}
	for _, f := range j.Files {
		path := filepath.Join(j.Artifact.ParentWorkspace, f.Path)
		if f.Missing {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(path, f.Before, f.Mode); err != nil {
				return err
			}
			if err := os.Chmod(path, f.Mode); err != nil {
				return err
			}
		}
	}
	return nil
}

// Recover returns integrations already applied and rolls back incomplete ones.
// Model/tool execution is deliberately never replayed after a process restart.
func (m *Manager) Recover(ctx context.Context, filters ...func(domain.AgentArtifact) bool) ([]domain.AgentArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(m.directory, "journals"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var completed []domain.AgentArtifact
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return completed, err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(m.directory, "journals", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return completed, err
		}
		var j journal
		if err = json.Unmarshal(data, &j); err != nil {
			return completed, err
		}
		if len(filters) > 0 && !filters[0](j.Artifact) {
			continue
		}
		if j.Completed {
			completed = append(completed, j.Artifact)
			continue
		}
		if err = restoreJournal(j); err != nil {
			return completed, err
		}
		if err = os.Remove(path); err != nil {
			return completed, err
		}
	}
	return completed, nil
}
func (m *Manager) Archive(ctx context.Context, a *domain.AgentArtifact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, err := filepath.Abs(m.directory)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(a.Workspace)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel != a.ID {
		return errors.New("unmanaged worktree")
	}
	if _, err = os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err = m.Capture(ctx, a); err != nil {
		return err
	}
	a.Status = "archived"
	if err = writeJournal(filepath.Join(m.directory, "archives", a.ID+".json"), journal{Artifact: *a, Completed: true}); err != nil {
		return err
	}
	_, err = git(ctx, a.ParentWorkspace, artifactGitEnv(*a), "worktree", "remove", "--force", path)
	return err
}
