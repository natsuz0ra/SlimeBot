package controller

import (
	"context"
	"errors"
	"net/http"

	"slimebot/internal/apperrors"
	"slimebot/internal/runtime/workspace"
)

type fileCheckpointService interface {
	ListFileCheckpoints(context.Context, string) ([]workspace.Checkpoint, error)
	GetFileCheckpoint(context.Context, string, string) (workspace.CheckpointDetail, error)
	DiffFileCheckpoint(context.Context, string, string, string) (workspace.CheckpointDiff, error)
	RollbackFileCheckpoint(context.Context, string, string, string) error
	CheckDeleteFileCheckpoints(string) error
	DeleteFileCheckpoints(string) error
	CancelBackground(string)
	ReserveSessionMaintenance(string) (func(), error)
	ReserveWorkspaceMaintenance(context.Context, string) (func(), error)
}

func (h *HTTPController) SetFileCheckpointService(service fileCheckpointService) {
	h.checkpoints = service
}

func (h *HTTPController) fileCheckpointsReady(c WebContext) bool {
	c.Writer().Header().Set("Cache-Control", "no-store")
	return true
}

func fileCheckpointError(c WebContext, err error) {
	switch {
	case errors.Is(err, workspace.ErrCheckpointMissing), errors.Is(err, apperrors.ErrNotFound):
		jsonError(c, http.StatusNotFound, "File checkpoint or session not found.")
	case errors.Is(err, workspace.ErrCheckpointInvalid):
		jsonError(c, http.StatusBadRequest, "Select undo or restore.")
	case errors.Is(err, workspace.ErrCheckpointBusy), errors.Is(err, workspace.ErrCheckpointConflict):
		jsonError(c, http.StatusConflict, err.Error())
	default:
		jsonInternalError(c, err)
	}
}

func (h *HTTPController) ListFileCheckpoints(c WebContext) {
	if !h.fileCheckpointsReady(c) {
		return
	}
	items, err := h.checkpoints.ListFileCheckpoints(c.Request().Context(), c.Param("id"))
	if err != nil {
		fileCheckpointError(c, err)
		return
	}
	c.JSON(http.StatusOK, map[string]any{"checkpoints": items})
}

func (h *HTTPController) GetFileCheckpoint(c WebContext) {
	if !h.fileCheckpointsReady(c) {
		return
	}
	item, err := h.checkpoints.GetFileCheckpoint(c.Request().Context(), c.Param("id"), c.Param("checkpointId"))
	if err != nil {
		fileCheckpointError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *HTTPController) DiffFileCheckpoint(c WebContext) {
	if !h.fileCheckpointsReady(c) {
		return
	}
	if c.Query("path") == "" {
		jsonError(c, http.StatusBadRequest, "A file path is required.")
		return
	}
	item, err := h.checkpoints.DiffFileCheckpoint(c.Request().Context(), c.Param("id"), c.Param("checkpointId"), c.Query("path"))
	if err != nil {
		fileCheckpointError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *HTTPController) RollbackFileCheckpoint(c WebContext) {
	if !h.fileCheckpointsReady(c) {
		return
	}
	var input struct {
		Mode string `json:"mode"`
	}
	c.Request().Body = http.MaxBytesReader(c.Writer(), c.Request().Body, 4096)
	if !bindJSONOrBadRequest(c, &input, "Select undo or restore.") {
		return
	}
	if err := h.checkpoints.RollbackFileCheckpoint(c.Request().Context(), c.Param("id"), c.Param("checkpointId"), input.Mode); err != nil {
		fileCheckpointError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
