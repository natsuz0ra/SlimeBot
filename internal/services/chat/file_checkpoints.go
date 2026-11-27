package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
	"slimebot/internal/logging"
	"slimebot/internal/runtime/workspace"
	"slimebot/internal/sandbox"
)

func (s *ChatService) SetFileCheckpoints(service *workspace.CheckpointManager) {
	s.checkpoints = service
}

func (s *ChatService) beginFileCheckpoint(ctx context.Context, state *chatTurnState, requestID string, policy *sandbox.Policy) (func(), error) {
	if s.checkpoints == nil {
		return func() {}, nil
	}
	root, err := workspace.ResolveCheckpointRoot(policy.CWD())
	if err != nil {
		return func() {}, nil // The normal sandbox/tool path reports a missing workspace.
	}
	session := state.session
	capture := (session.Kind == domain.SessionKindChat || session.Kind == "") &&
		!constants.IsMessagePlatformSessionID(session.ID) && strings.TrimSpace(session.WorkingDirectory) != ""
	selected, selectedErr := workspace.ResolveCheckpointRoot(session.WorkingDirectory)
	capture = capture && selectedErr == nil && selected == root
	var ownPending string
	if capture {
		ownPending = session.ID
	}
	release, err := s.checkpoints.ReserveRoots(ctx, checkpointWriteRoots(policy, root), true, !capture, ownPending)
	if err != nil {
		return nil, err
	}
	if !capture {
		return release, nil
	}
	backgroundAtStart := s.workspaceHasCommands(root)
	scanCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	start, err := s.checkpoints.BeginCapture(scanCtx, session.ID, root, requestID, state.userMessageID, backgroundAtStart)
	cancel()
	if err != nil {
		release()
		return nil, err
	}
	return func() {
		defer release()
		finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer cancel()
		if err := start.Finish(finalCtx, root, backgroundAtStart || s.workspaceHasCommands(root)); err != nil {
			logging.Warn("file_checkpoint_save_failed", "session", session.ID, "err", err)
		}
	}, nil
}

func (s *ChatService) workspaceHasCommands(root string) bool {
	s.agent.readFilesMu.Lock()
	defer s.agent.readFilesMu.Unlock()
	for _, manager := range s.agent.processesBySess {
		for _, spec := range manager.RunningSpecs() {
			if spec.SandboxMode == "" || spec.SandboxMode == string(sandbox.ModeDangerFullAccess) {
				return true
			}
			if spec.SandboxMode == string(sandbox.ModeReadOnly) {
				continue
			}
			for _, dir := range spec.WritableRoots {
				resolved, err := workspace.ResolveCheckpointRoot(dir)
				if err != nil || workspace.PathsOverlap(root, resolved) {
					return true
				}
			}
		}
	}
	return false
}

func checkpointWriteRoots(policy *sandbox.Policy, root string) []string {
	if policy.Mode() == sandbox.ModeDangerFullAccess {
		return []string{workspace.AllWorkspacePaths}
	}
	roots := []string{root}
	for _, path := range policy.WritableRoots() {
		resolved, err := workspace.ResolveCheckpointRoot(path)
		if err != nil {
			// A writable directory can be created later in the turn.
			resolved = path
		}
		roots = append(roots, resolved)
	}
	return roots
}

func (s *ChatService) checkpointSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	if s.checkpoints == nil || constants.IsMessagePlatformSessionID(sessionID) {
		return nil, apperrors.ErrNotFound
	}
	session, err := s.store.GetSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if (session.Kind != domain.SessionKindChat && session.Kind != "") || strings.TrimSpace(session.WorkingDirectory) == "" {
		return nil, apperrors.ErrNotFound
	}
	return session, nil
}

func (s *ChatService) ListFileCheckpoints(ctx context.Context, id string) ([]workspace.Checkpoint, error) {
	if _, err := s.checkpointSession(ctx, id); err != nil {
		return nil, err
	}
	return s.checkpoints.List(id)
}

func (s *ChatService) GetFileCheckpoint(ctx context.Context, sessionID, id string) (workspace.CheckpointDetail, error) {
	if _, err := s.checkpointSession(ctx, sessionID); err != nil {
		return workspace.CheckpointDetail{}, err
	}
	return s.checkpoints.Detail(sessionID, id)
}

func (s *ChatService) DiffFileCheckpoint(ctx context.Context, sessionID, id, path string) (workspace.CheckpointDiff, error) {
	if _, err := s.checkpointSession(ctx, sessionID); err != nil {
		return workspace.CheckpointDiff{}, err
	}
	return s.checkpoints.Diff(sessionID, id, path)
}

func (s *ChatService) RollbackFileCheckpoint(ctx context.Context, sessionID, id, mode string) error {
	releaseSession, err := s.ReserveSessionMaintenance(sessionID)
	if err != nil {
		return err
	}
	defer releaseSession()
	session, err := s.checkpointSession(ctx, sessionID)
	if err != nil {
		return err
	}
	root, err := workspace.ResolveCheckpointRoot(session.WorkingDirectory)
	if err != nil {
		return fmt.Errorf("%w: workspace directory is unavailable", workspace.ErrCheckpointConflict)
	}
	release, err := s.checkpoints.ReserveRoots(ctx, []string{root}, false, false, sessionID)
	if err != nil {
		return err
	}
	defer release()
	if s.workspaceHasCommands(root) {
		return workspace.ErrCheckpointBusy
	}
	policy, err := s.resolveSandboxPolicy(ctx, root)
	if err != nil {
		return err
	}
	return s.checkpoints.Rollback(ctx, sessionID, root, id, mode, policy.CheckWrite)
}

func (s *ChatService) ReserveSessionMaintenance(sessionID string) (func(), error) {
	release, err := s.acquireTurn(sessionID)
	if errors.Is(err, ErrTurnBusy) {
		return nil, workspace.ErrCheckpointBusy
	}
	return release, err
}

func (s *ChatService) ReserveWorkspaceMaintenance(ctx context.Context, directory string) (func(), error) {
	if s.checkpoints == nil || directory == "" {
		return func() {}, nil
	}
	root, err := workspace.ResolveCheckpointRoot(directory)
	if err != nil {
		return nil, err
	}
	return s.checkpoints.Reserve(ctx, root, false)
}

func (s *ChatService) CheckDeleteFileCheckpoints(sessionID string) error {
	if s.checkpoints == nil {
		return nil
	}
	return s.checkpoints.CheckDeleteSession(sessionID)
}

func (s *ChatService) DeleteFileCheckpoints(sessionID string) error {
	if s.checkpoints == nil {
		return nil
	}
	return s.checkpoints.DeleteSession(sessionID)
}
