package controllers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 校验账号并签发 JWT，返回前端会话所需鉴权信息。
func (h *HTTPController) Login(c *gin.Context) {
	if h.tokenManager == nil {
		jsonError(c, http.StatusInternalServerError, "鉴权服务未初始化")
		return
	}

	var req loginRequest
	if !bindJSONOrBadRequest(c, &req, "参数格式错误") {
		return
	}
	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		jsonError(c, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}

	storedUsername, err := h.repo.GetSetting(auth.SettingAuthUsername)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	storedHash, err := h.repo.GetSetting(auth.SettingAuthPasswordHash)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if storedUsername == "" || storedHash == "" {
		jsonError(c, http.StatusInternalServerError, "账号尚未初始化")
		return
	}
	if username != storedUsername || !auth.ComparePassword(storedHash, password) {
		jsonError(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	token, err := h.tokenManager.Generate(storedUsername)
	if err != nil {
		jsonError(c, http.StatusInternalServerError, "签发 token 失败")
		return
	}

	mustChangePassword, err := h.repo.GetSettingBool(auth.SettingAuthForcePasswordChange, false)
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
	if !bindJSONOrBadRequest(c, &req, "参数格式错误") {
		return
	}

	newUsername := strings.TrimSpace(req.Username)
	newPassword := strings.TrimSpace(req.NewPassword)
	if newUsername == "" && newPassword == "" {
		jsonError(c, http.StatusBadRequest, "至少修改用户名或密码中的一项")
		return
	}

	if newUsername != "" {
		if err := h.repo.SetSetting(auth.SettingAuthUsername, newUsername); err != nil {
			jsonInternalError(c, err)
			return
		}
	}

	if newPassword != "" {
		// 密码更新必须满足：提供旧密码、旧密码正确、且新旧密码不同。
		storedHash, err := h.repo.GetSetting(auth.SettingAuthPasswordHash)
		if err != nil {
			jsonInternalError(c, err)
			return
		}
		if storedHash == "" {
			jsonError(c, http.StatusInternalServerError, "账号尚未初始化")
			return
		}
		if strings.TrimSpace(req.OldPassword) == "" {
			jsonError(c, http.StatusBadRequest, "修改密码需要提供旧密码")
			return
		}
		if !auth.ComparePassword(storedHash, req.OldPassword) {
			jsonError(c, http.StatusBadRequest, "旧密码错误")
			return
		}
		if auth.ComparePassword(storedHash, newPassword) {
			jsonError(c, http.StatusBadRequest, "新密码不能与旧密码相同")
			return
		}

		hashed, err := auth.HashPassword(newPassword)
		if err != nil {
			jsonError(c, http.StatusInternalServerError, "密码加密失败")
			return
		}
		if err := h.repo.SetSetting(auth.SettingAuthPasswordHash, hashed); err != nil {
			jsonInternalError(c, err)
			return
		}
		if err := h.repo.SetSetting(auth.SettingAuthForcePasswordChange, "false"); err != nil {
			jsonInternalError(c, err)
			return
		}
	}

	c.Status(http.StatusNoContent)
}
