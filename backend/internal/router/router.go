package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"slimebot/backend/internal/auth"
	"slimebot/backend/internal/config"
	"slimebot/backend/internal/controllers"
	"slimebot/backend/internal/middleware"
)

func New(cfg config.Config, tokenManager *auth.TokenManager, httpController *controllers.HTTPController, wsController *controllers.WSController) *gin.Engine {
	r := gin.Default()
	r.Use(cors(cfg.Frontend))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	publicAPI := r.Group("/api")
	{
		publicAPI.POST("/login", httpController.Login)
	}

	protectedAPI := r.Group("/api")
	protectedAPI.Use(middleware.RequireJWT(tokenManager))
	{
		protectedAPI.PUT("/account", httpController.UpdateAccount)

		protectedAPI.GET("/sessions", httpController.ListSessions)
		protectedAPI.POST("/sessions", httpController.CreateSession)
		protectedAPI.PATCH("/sessions/:id/name", httpController.RenameSession)
		protectedAPI.DELETE("/sessions/:id", httpController.DeleteSession)
		protectedAPI.GET("/sessions/:id/messages", httpController.ListMessages)
		protectedAPI.PUT("/sessions/:id/model", httpController.SetSessionModel)

		protectedAPI.GET("/settings", httpController.GetSettings)
		protectedAPI.PUT("/settings", httpController.UpdateSettings)

		protectedAPI.GET("/llm-configs", httpController.ListLLMConfigs)
		protectedAPI.POST("/llm-configs", httpController.CreateLLMConfig)
		protectedAPI.DELETE("/llm-configs/:id", httpController.DeleteLLMConfig)

		protectedAPI.GET("/mcp-configs", httpController.ListMCPConfigs)
		protectedAPI.POST("/mcp-configs", httpController.CreateMCPConfig)
		protectedAPI.PUT("/mcp-configs/:id", httpController.UpdateMCPConfig)
		protectedAPI.DELETE("/mcp-configs/:id", httpController.DeleteMCPConfig)

		protectedAPI.GET("/skills", httpController.ListSkills)
		protectedAPI.POST("/skills/upload", httpController.UploadSkills)
		protectedAPI.DELETE("/skills/:id", httpController.DeleteSkill)
	}

	r.GET("/ws/chat", middleware.RequireJWT(tokenManager), func(c *gin.Context) {
		wsController.Chat(c.Writer, c.Request)
	})
	return r
}

func cors(allowOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
