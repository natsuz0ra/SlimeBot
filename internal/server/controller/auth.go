package controller

import (
	"errors"
	"net/http"
	"strings"

	authsvc "slimebot/internal/services/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login validates credentials and issues a JWT for the web client.
func (h *HTTPController) Login(c WebContext) {
	if h.tokenManager == nil {
		jsonError(c, http.StatusInternalServerError, "Authentication service is not initialized.")
		return
	}

	var req loginRequest
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}
	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		jsonError(c, http.StatusBadRequest, "Username and password are required.")
		return
	}

	ok, err := h.auth.VerifyLogin(c.Request().Context(), username, password)
	if err != nil {
		switch {
		case errors.Is(err, authsvc.ErrAccountNotInitialized):
			jsonError(c, http.StatusInternalServerError, "Account is not initialized.")
		default:
			jsonInternalError(c, err)
		}
		return
	}
	if !ok {
		jsonError(c, http.StatusUnauthorized, "Invalid username or password.")
		return
	}

	token, err := h.tokenManager.Generate(username)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Failed to issue token.")
		return
	}

	mustChangePassword, err := h.auth.MustChangePassword(c.Request().Context())
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, map[string]any{
		"token":              token,
		"tokenType":          "Bearer",
		"expiresInMinutes":   h.tokenManager.ExpireMinutes(),
		"mustChangePassword": mustChangePassword,
	})
}

// UpdateAccount updates username/password; password changes require the current password.
func (h *HTTPController) UpdateAccount(c WebContext) {
	var req struct {
		Username    string `json:"username"`
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if !bindJSONOrBadRequest(c, &req, "Invalid request payload format.") {
		return
	}

	newUsername := strings.TrimSpace(req.Username)
	newPassword := strings.TrimSpace(req.NewPassword)
	if newUsername == "" && newPassword == "" {
		jsonError(c, http.StatusBadRequest, "At least one of username or password must be updated.")
		return
	}

	err := h.auth.UpdateAccount(c.Request().Context(), req.Username, req.OldPassword, req.NewPassword)
	if err == nil {
		c.Status(http.StatusNoContent)
		return
	}
	switch {
	case errors.Is(err, authsvc.ErrAccountNotInitialized):
		jsonError(c, http.StatusInternalServerError, "Account is not initialized.")
	case errors.Is(err, authsvc.ErrOldPasswordRequired):
		jsonError(c, http.StatusBadRequest, "Current password is required to change password.")
	case errors.Is(err, authsvc.ErrOldPasswordInvalid):
		jsonError(c, http.StatusBadRequest, "Current password is incorrect.")
	case errors.Is(err, authsvc.ErrPasswordUnchanged):
		jsonError(c, http.StatusBadRequest, "New password must be different from the current password.")
	default:
		jsonInternalError(c, err)
	}
}
