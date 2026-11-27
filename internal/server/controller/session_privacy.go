package controller

import (
	"errors"
	"net/http"

	"slimebot/internal/apperrors"
)

func (h *HTTPController) DestroySessionPrivacyMappings(c WebContext) {
	if !h.privacyReady(c) {
		return
	}
	ctx := c.Request().Context()
	id := c.Param("id")
	if h.checkpoints != nil {
		release, err := h.checkpoints.ReserveSessionMaintenance(id)
		if err != nil {
			fileCheckpointError(c, err)
			return
		}
		defer release()
	}
	if _, err := h.sessions.Get(ctx, id); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			jsonError(c, http.StatusNotFound, "Session not found.")
		} else {
			jsonInternalError(c, err)
		}
		return
	}
	if err := h.privacy.DestroySession(id); err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
