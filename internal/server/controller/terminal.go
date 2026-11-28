package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"slimebot/internal/constants"
	sessionsvc "slimebot/internal/services/session"
	settingssvc "slimebot/internal/services/settings"
	"slimebot/internal/services/terminal"
)

type terminalSettings interface {
	Terminal(context.Context) (settingssvc.TerminalSettings, error)
	UpdateTerminal(context.Context, *bool, *string) error
}

func (h *HTTPController) SetTerminalService(manager *terminal.Manager, settings terminalSettings) {
	h.terminal, h.terminalSettings = manager, settings
}

func terminalOwner(c WebContext) string {
	owner, _ := c.Request().Context().Value(constants.ContextAuthUsername).(string)
	return owner
}

func (h *HTTPController) GetTerminalStatus(c WebContext) {
	policy, err := h.terminalSettings.Terminal(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, struct {
		settingssvc.TerminalSettings
		DetectedShell string                 `json:"detectedShell"`
		MaxSessions   int                    `json:"maxSessions"`
		Sessions      []terminal.SessionInfo `json:"sessions"`
	}{policy, terminal.DetectShell(policy.DefaultShell), terminal.MaxSessions, h.terminal.List(terminalOwner(c))})
}

func (h *HTTPController) UpdateTerminalSettings(c WebContext) {
	var req struct {
		Enabled      *bool   `json:"enabled"`
		DefaultShell *string `json:"defaultShell"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid terminal settings.") {
		return
	}
	if req.DefaultShell != nil && (len(*req.DefaultShell) > 2048 || strings.ContainsAny(*req.DefaultShell, "\x00\r\n")) {
		c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid default shell."})
		return
	}
	if err := h.terminalSettings.UpdateTerminal(c.Request().Context(), req.Enabled, req.DefaultShell); err != nil {
		jsonInternalError(c, err)
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		h.terminal.StopSessions()
	}
	c.Status(http.StatusNoContent)
}

func (h *HTTPController) CreateTerminal(c WebContext) {
	ctx := c.Request().Context()
	policy, err := h.terminalSettings.Terminal(ctx)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if !policy.Enabled {
		c.JSON(http.StatusForbidden, map[string]string{"message": "Terminal is disabled."})
		return
	}
	var opts terminal.Options
	if !bindJSONOrBadRequest(c, &opts, "Invalid terminal options.") {
		return
	}
	if opts.ChatSessionID != "" {
		session, err := h.sessions.Get(ctx, opts.ChatSessionID)
		if err != nil {
			jsonInternalError(c, err)
			return
		}
		if strings.TrimSpace(opts.Cwd) == "" {
			opts.Cwd = session.WorkingDirectory
		}
	}
	if strings.TrimSpace(opts.Cwd) != "" {
		opts.Cwd, err = sessionsvc.ValidateWorkingDirectory(opts.Cwd)
		if err != nil {
			c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
	} else {
		opts.Cwd = ""
	}
	info, err := h.terminal.Create(terminalOwner(c), opts, policy.DefaultShell)
	switch {
	case errors.Is(err, terminal.ErrQuota):
		c.JSON(http.StatusTooManyRequests, map[string]string{"message": err.Error()})
	case errors.Is(err, terminal.ErrSize):
		c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
	case err != nil:
		jsonInternalError(c, err)
	default:
		c.JSON(http.StatusCreated, info)
	}
}

func (h *HTTPController) DeleteTerminal(c WebContext) {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()
	err := h.terminal.Delete(ctx, terminalOwner(c), c.Param("id"))
	if errors.Is(err, terminal.ErrNotFound) {
		c.JSON(http.StatusNotFound, map[string]string{"message": err.Error()})
		return
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
