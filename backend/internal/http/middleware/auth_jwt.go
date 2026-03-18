package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/auth"
	"slimebot/backend/internal/consts"
)

func RequireJWT(tokenManager *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if tokenManager == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Authentication service is not initialized."})
			return
		}

		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized."})
			return
		}

		claims, err := tokenManager.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Token is invalid or expired."})
			return
		}

		c.Set(consts.ContextAuthUsername, claims.Username)
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	return strings.TrimSpace(c.Query("token"))
}
