package router

import (
	"net/http"

	"corner/backend/internal/config"
	"corner/backend/internal/controllers"
	"github.com/gin-gonic/gin"
)

func New(cfg config.Config, httpController *controllers.HTTPController, wsController *controllers.WSController) *gin.Engine {
	r := gin.Default()
	r.Use(cors(cfg.Frontend))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	{
		api.GET("/sessions", httpController.ListSessions)
		api.POST("/sessions", httpController.CreateSession)
		api.PATCH("/sessions/:id/name", httpController.RenameSession)
		api.DELETE("/sessions/:id", httpController.DeleteSession)
		api.GET("/sessions/:id/messages", httpController.ListMessages)
		api.PUT("/sessions/:id/model", httpController.SetSessionModel)

		api.GET("/settings", httpController.GetSettings)
		api.PUT("/settings", httpController.UpdateSettings)

		api.GET("/llm-configs", httpController.ListLLMConfigs)
		api.POST("/llm-configs", httpController.CreateLLMConfig)
		api.DELETE("/llm-configs/:id", httpController.DeleteLLMConfig)

		api.GET("/mcp-configs", httpController.ListMCPConfigs)
		api.POST("/mcp-configs", httpController.CreateMCPConfig)
		api.PUT("/mcp-configs/:id", httpController.UpdateMCPConfig)
		api.DELETE("/mcp-configs/:id", httpController.DeleteMCPConfig)

		api.GET("/skills", httpController.ListSkills)
		api.POST("/skills/upload", httpController.UploadSkills)
		api.DELETE("/skills/:id", httpController.DeleteSkill)
	}

	r.GET("/ws/chat", func(c *gin.Context) {
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
