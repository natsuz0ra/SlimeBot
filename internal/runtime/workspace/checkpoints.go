package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	checkpointHistoryLimit = 20
	checkpointDiskLimit    = (checkpointDeltaLimit + 2<<20) * 2
)

var (
	ErrCheckpointBusy     = errors.New("file workspace is busy")
	ErrCheckpointConflict = errors.New("file history conflicts with the current workspace")
	ErrCheckpointMissing  = errors.New("file checkpoint not found")
	ErrCheckpointInvalid  = errors.New("invalid file checkpoint request")
)

type Checkpoint struct {
	ID            string    `json:"id"`
	RequestID     string    `json:"requestId,omitempty"`
	UserMessageID string    `json:"userMessageId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	Kind          string    `json:"kind"`
	SourceID      string    `json:"sourceId,omitempty"`
	FileCount     int       `json:"fileCount"`
	Limited       bool      `json:"limited,omitempty"`
	Unavailable   bool      `json:"unavailable,omitempty"`
}

type CheckpointFileChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type CheckpointDetail struct {
	Checkpoint
	Files []CheckpointFileChange `json:"files"`
}

type checkpointHeader struct {
	Checkpoint
	Root     string
	Identity string
	Sequence uint64
}

type checkpointRecord struct {
	checkpointHeader
	changes []changedFile
}

// CheckpointManager keeps an independent history of working-tree bytes. Capture
// and rollback leases share the same workspace exclusion.
type CheckpointManager struct {
	directory   string
	mu          sync.Mutex
	active      map[*workspaceLease]bool
	changed     chan struct{}
	pending     map[string]string // pending journal path -> canonical workspace root
	pendingOnce sync.Once
	pendingErr  error
}

type workspaceLease struct {
	roots  []string
	shared bool
}

const AllWorkspacePaths = "*"

func NewCheckpoints(directory string) *CheckpointManager {
	if absolute, err := filepath.Abs(directory); err == nil {
		directory = absolute
	}
	return &CheckpointManager{directory: directory, active: make(map[*workspaceLease]bool), changed: make(chan struct{}), pending: make(map[string]string)}
}

func (m *CheckpointManager) Reserve(ctx context.Context, root string, wait bool, untracked ...bool) (func(), error) {
	return m.ReserveRoots(ctx, []string{root}, wait, len(untracked) > 0 && untracked[0], "")
}

// ReserveRoots atomically reserves all paths a turn may write, and never
// allows another session to run on top of a partially applied rollback.
func (m *CheckpointManager) ReserveRoots(ctx context.Context, roots []string, wait, shared bool, sessionID string) (func(), error) {
	m.pendingOnce.Do(m.loadPending)
	if m.pendingErr != nil {
		return nil, m.pendingErr
	}
	own := m.pendingPath(sessionID)
	lease := &workspaceLease{roots, shared}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.Lock()
		busy := false
		for _, root := range roots {
			for path, pendingRoot := range m.pending {
				if path != own && PathsOverlap(root, pendingRoot) {
					m.mu.Unlock()
					return nil, fmt.Errorf("%w: this workspace has an interrupted rollback in another session", ErrCheckpointConflict)
				}
			}
			for item := range m.active {
				if !(shared && item.shared) && slices.ContainsFunc(item.roots, func(active string) bool { return PathsOverlap(root, active) }) {
					busy = true
				}
			}
		}
		if !busy {
			m.active[lease] = true
			m.mu.Unlock()
			return func() {
				m.mu.Lock()
				delete(m.active, lease)
				close(m.changed)
				m.changed = make(chan struct{})
				m.mu.Unlock()
			}, nil
		}
		changed := m.changed
		m.mu.Unlock()
		if !wait {
			return nil, ErrCheckpointBusy
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (m *CheckpointManager) pendingPath(id string) string {
	return filepath.Join(m.sessionPath(id), "pending.json")
}

func (m *CheckpointManager) loadPending() {
	entries, err := os.ReadDir(m.directory)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		m.pendingErr = err
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) != sha256.Size*2 {
			continue
		}
		path := filepath.Join(m.directory, entry.Name(), "pending.json")
		record, err := readCheckpoint(path, false)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !filepath.IsAbs(record.Root) {
			m.pendingErr = errors.New("could not read an interrupted file rollback journal")
			return
		}
		m.pending[path] = record.Root
	}
}

func (m *CheckpointManager) updatePending(path, root string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if root == "" {
		delete(m.pending, path)
	} else {
		m.pending[path] = root
	}
}

func (m *CheckpointManager) sessionPath(sessionID string) string {
	id := sha256.Sum256([]byte(sessionID))
	return filepath.Join(m.directory, hex.EncodeToString(id[:]))
}

func readCheckpoint(path string, includeChanges bool) (checkpointRecord, error) {
	var record checkpointRecord
	f, err := os.Open(path)
	if err != nil {
		return record, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, checkpointDiskLimit))
	if err = dec.Decode(&record.checkpointHeader); err != nil {
		return record, err
	}
	if includeChanges {
		if err = dec.Decode(&record.changes); err != nil {
			return record, err
		}
		if len(record.changes) != record.FileCount {
			return record, errors.New("invalid checkpoint file count")
		}
		if err = validateChanges(record.changes); err != nil {
			return record, err
		}
	}
	return record, nil
}

func (m *CheckpointManager) headers(sessionID string) ([]checkpointHeader, error) {
	headers := make([]checkpointHeader, 0)
	dir := m.sessionPath(sessionID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return headers, nil
	}
	if err != nil {
		return nil, err
	}
	for _, item := range entries {
		id := strings.TrimSuffix(item.Name(), ".json")
		if item.IsDir() || id == item.Name() || !validCheckpointID(id) {
			continue
		}
		record, err := readCheckpoint(filepath.Join(dir, item.Name()), false)
		if errors.Is(err, fs.ErrNotExist) { // Concurrent retention or session deletion.
			continue
		}
		if err != nil {
			return nil, err
		}
		if record.ID != id || record.Sequence == 0 {
			return nil, errors.New("invalid checkpoint header")
		}
		headers = append(headers, record.checkpointHeader)
	}
	slices.SortFunc(headers, func(a, b checkpointHeader) int {
		if a.Sequence > b.Sequence {
			return -1
		}
		if a.Sequence < b.Sequence {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return headers, nil
}

func (m *CheckpointManager) List(sessionID string) ([]Checkpoint, error) {
	headers, err := m.headers(sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]Checkpoint, 0, len(headers))
	for _, h := range headers {
		result = append(result, h.Checkpoint)
	}
	return result, nil
}

func validCheckpointID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

func (m *CheckpointManager) record(sessionID, id string) (checkpointRecord, error) {
	if !validCheckpointID(id) {
		return checkpointRecord{}, ErrCheckpointMissing
	}
	record, err := readCheckpoint(filepath.Join(m.sessionPath(sessionID), id+".json"), true)
	if errors.Is(err, fs.ErrNotExist) {
		return record, ErrCheckpointMissing
	}
	if err == nil && record.ID != id {
		return record, errors.New("invalid checkpoint ID")
	}
	return record, err
}

func (m *CheckpointManager) Detail(sessionID, id string) (CheckpointDetail, error) {
	record, err := m.record(sessionID, id)
	if err != nil {
		return CheckpointDetail{}, err
	}
	result := CheckpointDetail{Checkpoint: record.Checkpoint, Files: make([]CheckpointFileChange, 0, len(record.changes))}
	for _, f := range record.changes {
		kind := "modified"
		switch {
		case f.Before == nil:
			kind = "added"
		case f.After == nil:
			kind = "deleted"
		}
		result.Files = append(result.Files, CheckpointFileChange{Path: f.Path, Kind: kind})
	}
	return result, nil
}

func (m *CheckpointManager) CheckDeleteSession(sessionID string) error {
	if _, err := os.Lstat(m.pendingPath(sessionID)); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%w: resolve the interrupted rollback before deleting this session", ErrCheckpointConflict)
	}
	return nil
}

func (m *CheckpointManager) DeleteSession(sessionID string) error {
	if err := m.CheckDeleteSession(sessionID); err != nil {
		return err
	}
	return os.RemoveAll(m.sessionPath(sessionID))
}

func (m *CheckpointManager) newRecord(sessionID, root, kind string) (checkpointRecord, error) {
	headers, err := m.headers(sessionID)
	if err != nil {
		return checkpointRecord{}, err
	}
	sequence := uint64(1)
	if len(headers) != 0 {
		sequence = headers[0].Sequence + 1
	}
	return checkpointRecord{checkpointHeader: checkpointHeader{
		Checkpoint: Checkpoint{ID: uuid.NewString(), Kind: kind, CreatedAt: time.Now().UTC()},
		Root:       root, Sequence: sequence,
	}}, nil
}

func writeCheckpoint(path string, record checkpointRecord) error {
	return writeWorkspaceState(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		if err := enc.Encode(record.checkpointHeader); err != nil {
			return err
		}
		return enc.Encode(record.changes)
	})
}

func (m *CheckpointManager) prune(sessionID string) error {
	headers, err := m.headers(sessionID)
	if err != nil || len(headers) <= checkpointHistoryLimit {
		return err
	}
	for _, old := range headers[checkpointHistoryLimit:] {
		if err := os.Remove(filepath.Join(m.sessionPath(sessionID), old.ID+".json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

type CheckpointCapture struct {
	manager   *CheckpointManager
	sessionID string
	record    checkpointRecord
	root      *os.Root
	before    fileSnapshot
	beforeErr error
}

// BeginCapture and Finish run while the caller holds its workspace lease.
func (m *CheckpointManager) BeginCapture(ctx context.Context, sessionID, rootPath, requestID, messageID string, background ...bool) (*CheckpointCapture, error) {
	backgroundRunning := len(background) > 0 && background[0]
	if !backgroundRunning {
		if err := m.Recover(sessionID, rootPath); err != nil {
			return nil, err
		}
	} else if _, err := os.Lstat(m.pendingPath(sessionID)); !errors.Is(err, fs.ErrNotExist) {
		return nil, ErrCheckpointBusy
	}
	record, err := m.newRecord(sessionID, rootPath, "turn")
	if err != nil {
		return nil, err
	}
	record.RequestID, record.UserMessageID = requestID, messageID
	record.Unavailable = true // A crash before Finish leaves a visible history gap.
	capture := &CheckpointCapture{manager: m, sessionID: sessionID, record: record}
	if err := writeCheckpoint(filepath.Join(m.sessionPath(sessionID), record.ID+".json"), record); err != nil {
		return nil, err
	}
	if err := m.prune(sessionID); err != nil {
		return nil, err
	}
	if backgroundRunning {
		capture.beforeErr = ErrCheckpointBusy
		return capture, nil
	}
	capture.root, capture.beforeErr = os.OpenRoot(rootPath)
	if capture.beforeErr == nil {
		capture.record.Identity, capture.beforeErr = checkpointIdentity(capture.root)
	}
	if capture.beforeErr == nil {
		capture.before, capture.beforeErr = scanCheckpoint(ctx, capture.root, m.directory)
	}
	return capture, nil
}

func (c *CheckpointCapture) Finish(ctx context.Context, rootPath string, inconsistent bool) error {
	if c.root != nil {
		defer c.root.Close()
	}
	record := c.record
	record.Unavailable = c.beforeErr != nil || inconsistent
	if !record.Unavailable {
		opened, openErr := c.root.Stat(".")
		current, statErr := os.Stat(rootPath)
		if openErr != nil || statErr != nil || !os.SameFile(opened, current) {
			record.Unavailable = true
		} else if after, err := scanCheckpoint(ctx, c.root, c.manager.directory); err != nil || after.git != c.before.git {
			record.Unavailable = true
		} else {
			record.changes, record.Limited = compareCheckpoints(c.root, c.before, after)
			if validateChanges(record.changes) != nil {
				record.Unavailable = true
				record.changes = nil
			}
		}
	}
	record.FileCount = len(record.changes)
	if err := writeCheckpoint(filepath.Join(c.manager.sessionPath(c.sessionID), record.ID+".json"), record); err != nil {
		return err
	}
	return c.manager.prune(c.sessionID)
}

// Rollback reverses one delta or the continuous sequence after a checkpoint.
// It persists a journal before touching the workspace; an interrupted write is
// compensated with the same preconditions on the next mutation.
func (m *CheckpointManager) Rollback(ctx context.Context, sessionID, rootPath, id, mode string, checkWrite func(string) error) error {
	if mode != "undo" && mode != "restore" {
		return ErrCheckpointInvalid
	}
	if err := m.Recover(sessionID, rootPath); err != nil {
		return err
	}
	target, err := m.record(sessionID, id)
	if err != nil {
		return err
	}
	if target.Root != rootPath || target.Unavailable {
		return fmt.Errorf("%w: the selected checkpoint has no usable snapshot", ErrCheckpointConflict)
	}
	changes, err := m.rollbackChanges(sessionID, target, mode)
	if err != nil || len(changes) == 0 {
		return err
	}
	if err := validateChanges(changes); err != nil {
		return fmt.Errorf("%w: %w", ErrCheckpointConflict, err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	identity, err := checkpointIdentity(root)
	if err != nil || target.Identity != identity {
		return fmt.Errorf("%w: the workspace directory was replaced", ErrCheckpointConflict)
	}
	for _, f := range changes {
		if checkWrite != nil {
			if err := checkWrite(filepath.Join(rootPath, filepath.FromSlash(f.Path))); err != nil {
				return fmt.Errorf("%w: %w", ErrCheckpointConflict, err)
			}
		}
		if err := checkCurrentFile(root, f.Path, f.Before); err != nil {
			return err
		}
	}
	record, err := m.newRecord(sessionID, rootPath, mode)
	if err != nil {
		return err
	}
	record.SourceID, record.Identity, record.changes, record.FileCount = id, identity, changes, len(changes)
	path := m.sessionPath(sessionID)
	pending := m.pendingPath(sessionID)
	if err := writeCheckpoint(pending, record); err != nil {
		return err
	}
	m.updatePending(pending, rootPath)
	applyErr := applyCheckpoint(ctx, root, changes)
	if applyErr == nil {
		applyErr = os.Rename(pending, filepath.Join(path, record.ID+".json"))
	}
	if applyErr != nil {
		return errors.Join(applyErr, m.Recover(sessionID, rootPath))
	}
	m.updatePending(pending, "")
	return m.prune(sessionID)
}

func applyCheckpoint(ctx context.Context, root *os.Root, changes []changedFile) error {
	for _, f := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkCurrentFile(root, f.Path, f.Before); err != nil {
			return err
		}
		if err := replaceCheckpointFile(root, f.Path, f.After); err != nil {
			return err
		}
	}
	return nil
}

func (m *CheckpointManager) rollbackChanges(sessionID string, target checkpointRecord, mode string) ([]changedFile, error) {
	if mode == "undo" {
		result := make([]changedFile, 0, len(target.changes))
		for _, f := range target.changes {
			result = append(result, changedFile{f.Path, f.After, f.Before})
		}
		return result, nil
	}
	headers, err := m.headers(sessionID)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]changedFile)
	for _, h := range headers {
		if h.Sequence <= target.Sequence {
			break
		}
		if h.Root != target.Root || h.Identity != target.Identity || h.Unavailable {
			return nil, fmt.Errorf("%w: file history is incomplete after the selected checkpoint", ErrCheckpointConflict)
		}
		record, err := m.record(sessionID, h.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range record.changes {
			combined, exists := byPath[f.Path]
			if !exists {
				byPath[f.Path] = changedFile{f.Path, f.After, f.Before}
				continue
			}
			if !fileEqual(combined.After, f.After) {
				return nil, fmt.Errorf("%w: %s changed between turns", ErrCheckpointConflict, f.Path)
			}
			combined.After = f.Before
			byPath[f.Path] = combined
		}
	}
	// Verify continuity at the selected turn too, where it captured this file.
	for _, f := range target.changes {
		if combined, exists := byPath[f.Path]; exists && !fileEqual(combined.After, f.After) {
			return nil, fmt.Errorf("%w: %s changed between turns", ErrCheckpointConflict, f.Path)
		}
	}
	result := make([]changedFile, 0, len(byPath))
	for _, f := range byPath {
		if !fileEqual(f.Before, f.After) {
			result = append(result, f)
		}
	}
	slices.SortFunc(result, func(a, b changedFile) int { return strings.Compare(a.Path, b.Path) })
	return result, nil
}

func (m *CheckpointManager) Recover(sessionID, rootPath string) error {
	pending := m.pendingPath(sessionID)
	record, err := readCheckpoint(pending, true)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.Root != rootPath {
		return fmt.Errorf("%w: interrupted rollback belongs to a different workspace", ErrCheckpointConflict)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	identity, err := checkpointIdentity(root)
	if err != nil || record.Identity != identity {
		return fmt.Errorf("%w: the workspace for the interrupted rollback was replaced", ErrCheckpointConflict)
	}
	corrections := make([]changedFile, 0, len(record.changes))
	for _, f := range record.changes {
		if checkCurrentFile(root, f.Path, f.Before) == nil {
			continue
		}
		if err := checkCurrentFile(root, f.Path, f.After); err != nil {
			return err
		}
		corrections = append(corrections, changedFile{f.Path, f.After, f.Before})
	}
	slices.Reverse(corrections)
	if err := applyCheckpoint(context.Background(), root, corrections); err != nil {
		return err
	}
	if err := os.Remove(pending); err != nil {
		return err
	}
	m.updatePending(pending, "")
	return nil
}
