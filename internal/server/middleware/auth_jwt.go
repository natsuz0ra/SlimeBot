package middleware

import (
	"context"
	"net/http"
	"strings"

	"slimebot/internal/auth"
	"slimebot/internal/constants"
	"slimebot/internal/logging"

	apierrors2 "slimebot/internal/server/apierrors"
)

// RequireJWT 校验请求中的 JWT，并把用户名写入请求上下文。
// 如果提供了 cliToken 且请求来自 localhost，则用 X-CLI-Token header 旁路认证。
func RequireJWT(tokenManager *auth.TokenManager, cliToken ...string) func(http.Handler) http.Handler {
	ct := ""
	if len(cliToken) > 0 {
		ct = cliToken[0]
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CLI token bypass: localhost + matching token → admin context
			if ct != "" && auth.IsLocalhost(r) {
				receivedToken := r.Header.Get("X-CLI-Token")
				logging.Info("cli_auth_check",
					"remote_addr", r.RemoteAddr,
					"received_token_length", len(receivedToken),
					"token_match", receivedToken == ct,
				)
				if receivedToken == ct {
					ctx := context.WithValue(r.Context(), constants.ContextAuthUsername, "admin")
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				logging.Warn("cli_token_mismatch",
					"remote_addr", r.RemoteAddr,
					"expected_length", len(ct),
					"received_length", len(receivedToken),
				)
			}

			if tokenManager == nil {
				logging.Error("auth_token_manager_nil")
				apierrors2.WriteJSONError(w, http.StatusInternalServerError, apierrors2.APIError{Message: "Authentication service is not initialized."})
				return
			}

			token := extractToken(r)
			if token == "" {
				logging.Warn("auth_empty_token", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
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
