package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	checkpointFileLimit  = 4 << 20
	checkpointScanLimit  = 64 << 20
	checkpointPathLimit  = 16000
	checkpointDeltaLimit = 8 << 20
)

var errSnapshotLimit = errors.New("workspace exceeds the checkpoint size limit")

type checkpointFile struct {
	Data []byte
	Mode fs.FileMode
}

type changedFile struct {
	Path          string
	Before, After *checkpointFile
}

type fileSnapshot struct {
	files   map[string]*checkpointFile // nil entries were observed but excluded
	ignored map[string]bool            // Git can report ignored folders as one entry.
	limited bool
	git     bool
}

func fileEqual(a, b *checkpointFile) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Mode == b.Mode && bytes.Equal(a.Data, b.Data)
}

func checkpointPath(name string) bool {
	if !fs.ValidPath(name) || name == "." || !utf8.ValidString(name) || strings.ContainsAny(name, "\\:\x00") || protected(name) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		switch strings.ToLower(part) {
		case ".git", ".hg", ".svn", ".slimebot", "node_modules", ".venv", ".ssh", ".aws", ".gnupg":
			return false
		}
	}
	base := strings.ToLower(filepath.Base(name))
	if base == ".npmrc" || base == ".pypirc" || base == ".netrc" || strings.HasPrefix(base, "credentials.") {
		return false
	}
	for _, suffix := range []string{".pem", ".key", ".pfx", ".p12", ".db-wal", ".db-shm", ".db-journal", ".sqlite-wal", ".sqlite-shm"} {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	return true
}

func ResolveCheckpointRoot(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("file checkpoints require a workspace directory")
	}
	return filepath.EvalSymlinks(path)
}

func PathsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == AllWorkspacePaths || b == AllWorkspacePaths {
		return true
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	return containsPath(a, b) || containsPath(b, a)
}

func containsPath(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	return err == nil && filepath.IsLocal(rel)
}

// inspectFile checks every path component and opens files through the bounded root.
// A present but unsupported path is distinct from an absent file.
func inspectFile(root *os.Root, name string) (file *checkpointFile, present bool, err error) {
	if !checkpointPath(name) {
		return nil, true, nil
	}
	local := filepath.FromSlash(name)
	for path := filepath.Dir(local); path != "."; path = filepath.Dir(path) {
		info, err := root.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		if !info.IsDir() {
			return nil, true, nil
		}
	}
	info, err := root.Lstat(local)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > checkpointFileLimit {
		return nil, true, nil
	}
	f, err := root.Open(local)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	if !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, true, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, checkpointFileLimit+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > checkpointFileLimit {
		return nil, true, nil
	}
	return &checkpointFile{Data: data, Mode: opened.Mode().Perm()}, true, nil
}

func scanCheckpoint(ctx context.Context, root *os.Root, storage string) (fileSnapshot, error) {
	result := fileSnapshot{files: make(map[string]*checkpointFile), ignored: make(map[string]bool)}
	paths, total := 0, 0
	visit := func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		paths++
		if paths > checkpointPathLimit {
			return errSnapshotLimit
		}
		if containsPath(storage, filepath.Join(root.Name(), filepath.FromSlash(name))) {
			result.limited = true
			return nil
		}
		file, present, err := inspectFile(root, name)
		if err != nil {
			return err
		}
		if present {
			result.files[name] = file
			if file == nil {
				result.limited = true
			} else {
				total += len(file.Data)
				if total > checkpointScanLimit {
					return errSnapshotLimit
				}
			}
		}
		return nil
	}
	if inside, err := git(ctx, root.Name(), nil, "rev-parse", "--is-inside-work-tree"); err == nil && inside == "true" {
		result.git = true
		names, err := gitRaw(ctx, root.Name(), nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--deduplicate", "--", ".")
		if err != nil {
			return result, err
		}
		for name := range strings.SplitSeq(names, "\x00") {
			if name != "" {
				if err := visit(name); err != nil {
					return result, err
				}
			}
		}
		ignored, err := gitRaw(ctx, root.Name(), nil, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory", "--", ".")
		if err != nil {
			return result, err
		}
		for name := range strings.SplitSeq(ignored, "\x00") {
			if name != "" {
				paths++
				if paths > checkpointPathLimit {
					return result, errSnapshotLimit
				}
				result.ignored[name] = true
			}
		}
		return result, nil
	}
	// Without Git, walk only this directory. Do not traverse nested repositories or links.
	err := fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		if d.IsDir() {
			if !checkpointPath(name) || containsPath(storage, filepath.Join(root.Name(), filepath.FromSlash(name))) {
				result.limited = true
				return fs.SkipDir
			}
			if _, err := root.Lstat(filepath.FromSlash(name + "/.git")); err == nil {
				result.limited = true
				result.ignored[name+"/"] = true
				return fs.SkipDir
			}
			paths++
			if paths > checkpointPathLimit {
				return errSnapshotLimit
			}
			return ctx.Err()
		}
		return visit(name)
	})
	return result, err
}

func compareCheckpoints(root *os.Root, before, after fileSnapshot) ([]changedFile, bool) {
	limited := before.limited || after.limited || before.git != after.git
	changes := make([]changedFile, 0)
	for name, old := range before.files {
		if old == nil {
			continue
		}
		if next, exists := after.files[name]; exists {
			if next != nil && !fileEqual(old, next) {
				changes = append(changes, changedFile{name, old, next})
			}
		} else if _, err := root.Lstat(filepath.FromSlash(name)); errors.Is(err, fs.ErrNotExist) {
			changes = append(changes, changedFile{name, old, nil})
		} else {
			// A changed ignore rule can hide a file still on disk.
			limited = true
		}
	}
	for name, next := range after.files {
		if _, exists := before.files[name]; !exists && next != nil {
			if before.wasIgnored(name) {
				// An unignored file existed before the turn; undo must not delete it.
				limited = true
				continue
			}
			changes = append(changes, changedFile{name, nil, next})
		}
	}
	slices.SortFunc(changes, func(a, b changedFile) int { return strings.Compare(a.Path, b.Path) })
	return changes, limited
}

func (s fileSnapshot) wasIgnored(path string) bool {
	if s.ignored[path] {
		return true
	}
	for i := strings.LastIndexByte(path, '/'); i >= 0; i = strings.LastIndexByte(path[:i], '/') {
		if s.ignored[path[:i+1]] {
			return true
		}
	}
	return false
}

func validateChanges(changes []changedFile) error {
	size := 0
	paths := make(map[string]bool, len(changes))
	for _, change := range changes {
		name := change.Path
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			name = strings.ToLower(name)
		}
		if !checkpointPath(change.Path) || paths[name] || fileEqual(change.Before, change.After) {
			return fmt.Errorf("%w: unsupported path %q", ErrCheckpointConflict, change.Path)
		}
		paths[name] = true
		size += len(change.Path)
		for _, file := range []*checkpointFile{change.Before, change.After} {
			if file != nil {
				if len(file.Data) > checkpointFileLimit || file.Mode != file.Mode.Perm() {
					return errSnapshotLimit
				}
				size += len(file.Data)
			}
		}
		if size > checkpointDeltaLimit || len(paths) > checkpointPathLimit {
			return errSnapshotLimit
		}
	}
	return nil
}

func checkCurrentFile(root *os.Root, name string, expected *checkpointFile) error {
	current, present, err := inspectFile(root, name)
	if err != nil || (present && current == nil) || !fileEqual(current, expected) {
		return fmt.Errorf("%w: %s", ErrCheckpointConflict, name)
	}
	return nil
}

func replaceCheckpointFile(root *os.Root, path string, file *checkpointFile) error {
	name := filepath.FromSlash(path)
	if file == nil {
		return root.Remove(name)
	}
	parent := filepath.Dir(name)
	if err := root.MkdirAll(parent, 0755); err != nil {
		return err
	}
	temp := filepath.Join(parent, ".slimebot-checkpoint-"+uuid.NewString())
	defer root.Remove(temp)
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(file.Data)
	if err == nil {
		err = f.Chmod(file.Mode)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return root.Rename(temp, name)
}
