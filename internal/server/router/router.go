package router

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"slimebot/internal/auth"
	"slimebot/internal/config"
	"slimebot/internal/server/controller"
	"slimebot/internal/server/middleware"
	"slimebot/internal/server/ws"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
)

// RouterConfig 路由级可选配置。
type RouterConfig struct {
	// CLIToken 如果非空，启用 CLI 本地 token 旁路认证。
	CLIToken string
	// Headless 如果为 true，跳过 SPA 静态文件路由。
	Headless bool
}

// New 构建 HTTP 路由树并注册 REST 与 WebSocket 入口。
func New(cfg config.Config, tokenManager *auth.TokenManager, httpController *controller.HTTPController, wsController *ws.Controller, staticFS fs.FS, routerCfg ...RouterConfig) http.Handler {
	var rc RouterConfig
	if len(routerCfg) > 0 {
		rc = routerCfg[0]
	}
	r := chi.NewRouter()
	if strings.TrimSpace(cfg.Frontend) != "" {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   []string{cfg.Frontend},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Content-Type", "Authorization"},
			AllowCredentials: true,
		}))
	}
	r.Use(chimiddleware.Compress(5))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	// REST API
	r.Route("/api", func(api chi.Router) {
		api.Use(httprate.LimitByIP(400, time.Minute))
		api.With(httprate.LimitByIP(30, time.Minute)).Post("/login", adapt(httpController.Login))

		authmw := middleware.RequireJWT(tokenManager, rc.CLIToken)
		api.With(authmw).Route("/", func(api chi.Router) {
			api.Put("/account", adapt(httpController.UpdateAccount))

			api.Get("/sessions", adapt(httpController.ListSessions))
			api.Post("/sessions", adapt(httpController.CreateSession))
			api.Patch("/sessions/{id}/name", adapt(httpController.RenameSession))
			api.Delete("/sessions/{id}", adapt(httpController.DeleteSession))
			api.Get("/sessions/{id}/messages", adapt(httpController.ListMessages))
			api.Post("/sessions/{id}/attachments", adapt(httpController.UploadSessionAttachments))

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

	// WebSocket
	wsAuthmw := middleware.RequireJWT(tokenManager, rc.CLIToken)
	r.With(wsAuthmw).Get("/ws/chat", func(w http.ResponseWriter, req *http.Request) {
		wsController.Chat(w, req)
	})

	// SPA 静态文件（headless 模式跳过）
	if !rc.Headless {
		r.Get("/*", serveSPA(staticFS))
	}

	return r
}

// adapt 将控制器的 WebContext 风格处理器适配为 net/http Handler。
func serveSPA(staticFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(staticFS, p); err != nil {
			p = "index.html"
		}
		http.ServeFileFS(w, r, staticFS, p)
	}
}

func adapt(fn func(controller.WebContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fn(controller.NewChiContext(w, r))
	}
}
