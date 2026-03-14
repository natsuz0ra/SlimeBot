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

func (h *HTTPController) Login(c *gin.Context) {
	if h.tokenManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "鉴权服务未初始化"})
		return
	}

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}
	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	storedUsername, err := h.repo.GetSetting(auth.SettingAuthUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	storedHash, err := h.repo.GetSetting(auth.SettingAuthPasswordHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if storedUsername == "" || storedHash == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "账号尚未初始化"})
		return
	}
	if username != storedUsername || !auth.ComparePassword(storedHash, password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	token, err := h.tokenManager.Generate(storedUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "签发 token 失败"})
		return
	}

	mustChangePassword, err := h.repo.GetSettingBool(auth.SettingAuthForcePasswordChange, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":              token,
		"tokenType":          "Bearer",
		"expiresInMinutes":   h.tokenManager.ExpireMinutes(),
		"mustChangePassword": mustChangePassword,
	})
}

func (h *HTTPController) UpdateAccount(c *gin.Context) {
	var req struct {
		Username    string `json:"username"`
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数格式错误"})
		return
	}

	newUsername := strings.TrimSpace(req.Username)
	newPassword := strings.TrimSpace(req.NewPassword)
	if newUsername == "" && newPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "至少修改用户名或密码中的一项"})
		return
	}

	if newUsername != "" {
		if err := h.repo.SetSetting(auth.SettingAuthUsername, newUsername); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if newPassword != "" {
		storedHash, err := h.repo.GetSetting(auth.SettingAuthPasswordHash)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if storedHash == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "账号尚未初始化"})
			return
		}
		if strings.TrimSpace(req.OldPassword) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "修改密码需要提供旧密码"})
			return
		}
		if !auth.ComparePassword(storedHash, req.OldPassword) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码错误"})
			return
		}
		if auth.ComparePassword(storedHash, newPassword) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "新密码不能与旧密码相同"})
			return
		}

		hashed, err := auth.HashPassword(newPassword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}
		if err := h.repo.SetSetting(auth.SettingAuthPasswordHash, hashed); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := h.repo.SetSetting(auth.SettingAuthForcePasswordChange, "false"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.Status(http.StatusNoContent)
}
