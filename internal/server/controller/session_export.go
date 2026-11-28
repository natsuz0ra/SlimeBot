package controller

import (
	"context"
	"mime"
	"net/http"
	"strconv"

	"slimebot/internal/domain"
	sessionsvc "slimebot/internal/services/session"
)

type sessionWorkbenchService interface {
	SetPinned(context.Context, string, bool) (*domain.Session, error)
	Export(context.Context, string, sessionsvc.ExportOptions) (sessionsvc.ExportFile, error)
}

func (h *HTTPController) SetSessionWorkbenchService(service sessionWorkbenchService) {
	h.sessionWorkbench = service
}

func (h *HTTPController) PinSession(c WebContext) {
	var req struct {
		Pinned *bool `json:"pinned"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid pin request.") {
		return
	}
	if req.Pinned == nil {
		jsonError(c, http.StatusBadRequest, "pinned is required.")
		return
	}
	value, err := h.sessionWorkbench.SetPinned(c.Request().Context(), c.Param("id"), *req.Pinned)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, value)
}

func (h *HTTPController) ExportSession(c WebContext) {
	opts := sessionsvc.ExportOptions{Format: c.Query("format"), IncludeTools: true}
	if opts.Format == "" {
		opts.Format = "markdown"
	}
	for key, target := range map[string]*bool{"includeTools": &opts.IncludeTools, "includeThinking": &opts.IncludeThinking} {
		if raw := c.Query(key); raw != "" {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				jsonError(c, http.StatusBadRequest, "Invalid export options.")
				return
			}
			*target = value
		}
	}
	file, err := h.sessionWorkbench.Export(c.Request().Context(), c.Param("id"), opts)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	w := c.Writer()
	w.Header().Set("Content-Type", file.MediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
	if _, err := w.Write(file.Data); err != nil {
		c.Error(err)
	}
}
