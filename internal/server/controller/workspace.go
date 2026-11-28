package controller

import (
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"slimebot/internal/services/workspace"
)

func (h *HTTPController) workspaceDirectory(c WebContext) (string, bool) {
	session, err := h.sessions.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		jsonInternalError(c, err)
		return "", false
	}
	if strings.TrimSpace(session.WorkingDirectory) == "" {
		jsonError(c, http.StatusBadRequest, "This session has no working directory.")
		return "", false
	}
	return session.WorkingDirectory, true
}

func workspaceResult(c WebContext, data any, err error) {
	switch {
	case err == nil:
		c.JSON(http.StatusOK, data)
	case errors.Is(err, workspace.ErrGitTimeout):
		jsonError(c, http.StatusGatewayTimeout, err.Error())
	case errors.Is(err, workspace.ErrInvalidPath), errors.Is(err, workspace.ErrNotFile):
		jsonError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, fs.ErrNotExist):
		jsonError(c, http.StatusNotFound, "Workspace path not found.")
	case errors.Is(err, fs.ErrPermission):
		jsonError(c, http.StatusForbidden, "Workspace path is inaccessible.")
	default:
		jsonInternalError(c, err)
	}
}

func workspaceFlag(c WebContext, key string) (bool, bool) {
	raw := c.Query(key)
	if raw == "" {
		return false, true
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid "+key+" option.")
		return false, false
	}
	return value, true
}

func (h *HTTPController) WorkspaceGitStatus(c WebContext) {
	if root, ok := h.workspaceDirectory(c); ok {
		data, err := workspace.Status(c.Request().Context(), root)
		workspaceResult(c, data, err)
	}
}

func (h *HTTPController) WorkspaceGitDiff(c WebContext) {
	staged, valid := workspaceFlag(c, "staged")
	if !valid {
		return
	}
	if root, ok := h.workspaceDirectory(c); ok {
		data, err := workspace.Diff(c.Request().Context(), root, c.Query("path"), staged)
		workspaceResult(c, data, err)
	}
}

func (h *HTTPController) WorkspaceTree(c WebContext) {
	showHidden, valid := workspaceFlag(c, "showHidden")
	if !valid {
		return
	}
	if root, ok := h.workspaceDirectory(c); ok {
		data, err := workspace.OpenTree(root, c.Query("path"), showHidden)
		workspaceResult(c, data, err)
	}
}

func (h *HTTPController) WorkspaceFile(c WebContext) {
	if root, ok := h.workspaceDirectory(c); ok {
		data, err := workspace.ReadFile(root, c.Query("path"))
		workspaceResult(c, data, err)
	}
}
