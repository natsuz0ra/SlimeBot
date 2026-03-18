package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/services"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 校验账号并签发 JWT，返回前端会话所需鉴权信息。
func (h *HTTPController) Login(c *gin.Context) {
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

	ok, err := h.auth.VerifyLogin(username, password)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if !ok {
		storedUsername, getErr := h.repo.GetSetting(consts.SettingAuthUsername)
		storedHash, hashErr := h.repo.GetSetting(consts.SettingAuthPasswordHash)
		if getErr != nil || hashErr != nil {
			jsonInternalError(c, err)
			return
		}
		if storedUsername == "" || storedHash == "" {
			jsonError(c, http.StatusInternalServerError, "Account is not initialized.")
			return
		}
		jsonError(c, http.StatusUnauthorized, "Invalid username or password.")
		return
	}

	token, err := h.tokenManager.Generate(username)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "Failed to issue token.")
		return
	}

	mustChangePassword, err := h.auth.MustChangePassword()
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":              token,
		"tokenType":          "Bearer",
		"expiresInMinutes":   h.tokenManager.ExpireMinutes(),
		"mustChangePassword": mustChangePassword,
	})
}

// UpdateAccount 更新账号信息；修改密码时会强制校验旧密码。
func (h *HTTPController) UpdateAccount(c *gin.Context) {
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

	err := h.auth.UpdateAccount(req.Username, req.OldPassword, req.NewPassword)
	if err == nil {
		c.Status(http.StatusNoContent)
		return
	}
	switch {
	case errors.Is(err, services.ErrAccountNotInitialized):
		jsonError(c, http.StatusInternalServerError, "Account is not initialized.")
	case errors.Is(err, services.ErrOldPasswordRequired):
		jsonError(c, http.StatusBadRequest, "Current password is required to change password.")
	case errors.Is(err, services.ErrOldPasswordInvalid):
		jsonError(c, http.StatusBadRequest, "Current password is incorrect.")
	case errors.Is(err, services.ErrPasswordUnchanged):
		jsonError(c, http.StatusBadRequest, "New password must be different from the current password.")
	default:
		jsonInternalError(c, err)
	}
}
