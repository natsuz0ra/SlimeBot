package router

import (
	"encoding/json"
	"net/http"

	"slimebot/backend/internal/auth"
	"slimebot/backend/internal/config"
	"slimebot/backend/internal/server/controller"
	"slimebot/backend/internal/server/middleware"
	"slimebot/backend/internal/server/ws"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// New 构建 HTTP 路由树并注册 REST 与 WebSocket 入口。
func New(cfg config.Config, tokenManager *auth.TokenManager, httpController *controller.HTTPController, wsController *ws.Controller) http.Handler {
	r := chi.NewRouter()
	r.Use(cors(cfg.Frontend))
	r.Use(chimiddleware.Compress(5))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// REST API
	r.Route("/api", func(api chi.Router) {
		api.Post("/login", adapt(httpController.Login))

		api.With(middleware.RequireJWT(tokenManager)).Route("/", func(api chi.Router) {
			api.Put("/account", adapt(httpController.UpdateAccount))

			api.Get("/sessions", adapt(httpController.ListSessions))
			api.Post("/sessions", adapt(httpController.CreateSession))
			api.Patch("/sessions/{id}/name", adapt(httpController.RenameSession))
			api.Delete("/sessions/{id}", adapt(httpController.DeleteSession))
			api.Get("/sessions/{id}/messages", adapt(httpController.ListMessages))
			api.Post("/sessions/{id}/attachments", adapt(httpController.UploadSessionAttachments))
			api.Put("/sessions/{id}/model", adapt(httpController.SetSessionModel))

			api.Get("/settings", adapt(httpController.GetSettings))
			api.Put("/settings", adapt(httpController.UpdateSettings))

			api.Get("/llm-configs", adapt(httpController.ListLLMConfigs))
			api.Post("/llm-configs", adapt(httpController.CreateLLMConfig))
			api.Delete("/llm-configs/{id}", adapt(httpController.DeleteLLMConfig))

			api.Get("/mcp-configs", adapt(httpController.ListMCPConfigs))
			api.Post("/mcp-configs", adapt(httpController.CreateMCPConfig))
			api.Put("/mcp-configs/{id}", adapt(httpController.UpdateMCPConfig))
			api.Delete("/mcp-configs/{id}", adapt(httpController.DeleteMCPConfig))

			api.Get("/message-platform-configs", adapt(httpController.ListMessagePlatformConfigs))
			api.Post("/message-platform-configs", adapt(httpController.CreateMessagePlatformConfig))
			api.Put("/message-platform-configs/{id}", adapt(httpController.UpdateMessagePlatformConfig))
			api.Delete("/message-platform-configs/{id}", adapt(httpController.DeleteMessagePlatformConfig))

			api.Get("/skills", adapt(httpController.ListSkills))
			api.Post("/skills/upload", adapt(httpController.UploadSkills))
			api.Delete("/skills/{id}", adapt(httpController.DeleteSkill))
		})
	})

	// WebSocket (net/http entrypoint)
	r.With(middleware.RequireJWT(tokenManager)).Get("/ws/chat", func(w http.ResponseWriter, req *http.Request) {
		wsController.Chat(w, req)
	})

	return r
}

// adapt 将控制器的 WebContext 风格处理器适配为 net/http Handler。
func adapt(fn func(controller.WebContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(controller.NewChiContext(w, r))
	}
}

// cors 返回基础跨域中间件，统一处理预检请求。
func cors(allowOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
