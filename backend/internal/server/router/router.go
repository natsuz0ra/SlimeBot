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
)

func New(cfg config.Config, tokenManager *auth.TokenManager, httpController *controller.HTTPController, wsController *ws.Controller) http.Handler {
	r := chi.NewRouter()
	r.Use(cors(cfg.Frontend))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// REST API
	r.Route("/api", func(api chi.Router) {
		api.Post("/login", adapt(httpController.Login))

		api.With(middleware.RequireJWT(tokenManager)).Route("/", func(protected chi.Router) {
			protected.Put("/account", adapt(httpController.UpdateAccount))

			protected.Get("/sessions", adapt(httpController.ListSessions))
			protected.Post("/sessions", adapt(httpController.CreateSession))
			protected.Patch("/sessions/{id}/name", adapt(httpController.RenameSession))
			protected.Delete("/sessions/{id}", adapt(httpController.DeleteSession))
			protected.Get("/sessions/{id}/messages", adapt(httpController.ListMessages))
			protected.Post("/sessions/{id}/attachments", adapt(httpController.UploadSessionAttachments))
			protected.Put("/sessions/{id}/model", adapt(httpController.SetSessionModel))

			protected.Get("/settings", adapt(httpController.GetSettings))
			protected.Put("/settings", adapt(httpController.UpdateSettings))

			protected.Get("/llm-configs", adapt(httpController.ListLLMConfigs))
			protected.Post("/llm-configs", adapt(httpController.CreateLLMConfig))
			protected.Delete("/llm-configs/{id}", adapt(httpController.DeleteLLMConfig))

			protected.Get("/mcp-configs", adapt(httpController.ListMCPConfigs))
			protected.Post("/mcp-configs", adapt(httpController.CreateMCPConfig))
			protected.Put("/mcp-configs/{id}", adapt(httpController.UpdateMCPConfig))
			protected.Delete("/mcp-configs/{id}", adapt(httpController.DeleteMCPConfig))

			protected.Get("/message-platform-configs", adapt(httpController.ListMessagePlatformConfigs))
			protected.Post("/message-platform-configs", adapt(httpController.CreateMessagePlatformConfig))
			protected.Put("/message-platform-configs/{id}", adapt(httpController.UpdateMessagePlatformConfig))
			protected.Delete("/message-platform-configs/{id}", adapt(httpController.DeleteMessagePlatformConfig))

			protected.Get("/skills", adapt(httpController.ListSkills))
			protected.Post("/skills/upload", adapt(httpController.UploadSkills))
			protected.Delete("/skills/{id}", adapt(httpController.DeleteSkill))
		})
	})

	// WebSocket (net/http entrypoint)
	r.With(middleware.RequireJWT(tokenManager)).Get("/ws/chat", func(w http.ResponseWriter, req *http.Request) {
		wsController.Chat(w, req)
	})

	return r
}

func adapt(fn func(controller.WebContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(controller.NewChiContext(w, r))
	}
}

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
