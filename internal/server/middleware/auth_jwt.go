package middleware

import (
	"context"
	"net/http"
	apierrors2 "slimebot/internal/server/apierrors"
	"strings"

	"slimebot/internal/auth"
	"slimebot/internal/constants"
)

// RequireJWT 校验请求中的 JWT，并把用户名写入请求上下文。
func RequireJWT(tokenManager *auth.TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tokenManager == nil {
				apierrors2.WriteJSONError(w, http.StatusInternalServerError, apierrors2.APIError{Message: "Authentication service is not initialized."})
				return
			}

			token := extractToken(r)
			if token == "" {
				apierrors2.WriteJSONError(w, http.StatusUnauthorized, apierrors2.APIError{Message: "Unauthorized."})
				return
			}

			claims, err := tokenManager.Parse(token)
			if err != nil {
				apierrors2.WriteJSONError(w, http.StatusUnauthorized, apierrors2.APIError{Message: "Token is invalid or expired."})
				return
			}

			ctx := context.WithValue(r.Context(), constants.ContextAuthUsername, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractToken 优先从 Authorization: Bearer 读取，失败时回退 query token。
func extractToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}
