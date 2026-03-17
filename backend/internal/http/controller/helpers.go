package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// jsonError 统一错误响应结构，避免各 handler 重复拼装。
func jsonError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

// jsonInternalError 统一 500 错误写出，并兼容空错误对象。
func jsonInternalError(c *gin.Context, err error) {
	if err != nil {
		_ = c.Error(err)
	}
	jsonError(c, http.StatusInternalServerError, "internal server error")
}

// bindJSONOrBadRequest 统一请求体绑定失败处理，返回 false 表示应立即结束 handler。
func bindJSONOrBadRequest(c *gin.Context, req any, message string) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		jsonError(c, http.StatusBadRequest, message)
		return false
	}
	return true
}
