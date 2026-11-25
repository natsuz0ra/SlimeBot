package chat

import (
	"context"
	"errors"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	contextsvc "slimebot/internal/services/context"
)

// contextRuntimeState reads bounded host-owned state for post-compaction recovery.
func (s *ChatService) contextRuntimeState(ctx context.Context, sessionID string) (contextsvc.RuntimeState, error) {
	var state contextsvc.RuntimeState
	if s.checkpoints != nil {
		checkpoints, err := s.ListFileCheckpoints(ctx, sessionID)
		if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
			return state, err
		}
		if len(checkpoints) > 0 {
			latestIndex := 0
			for i := range checkpoints {
				if checkpoints[i].CreatedAt.After(checkpoints[latestIndex].CreatedAt) {
					latestIndex = i
				}
			}
			detail, err := s.GetFileCheckpoint(ctx, sessionID, checkpoints[latestIndex].ID)
			if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
				return state, err
			} else if err == nil {
				for _, file := range detail.Files {
					state.Files = append(state.Files, contextsvc.RuntimeFileState{Path: file.Path, Kind: file.Kind, CheckpointID: detail.ID})
				}
			}
		}
	}
	tasks, err := s.ListBackgroundTasks(ctx, sessionID, 20)
	if err != nil {
		return state, err
	}
	for _, task := range tasks {
		switch task.Status {
		case domain.BackgroundStatusRunning, domain.BackgroundStatusQueued, domain.BackgroundStatusWaking, domain.BackgroundStatusFailed:
			state.Tasks = append(state.Tasks, contextsvc.RuntimeTaskState{ID: task.ID, Status: task.Status, Command: task.Command, Error: task.Error})
		}
	}
	return state, nil
}
