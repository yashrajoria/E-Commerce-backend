package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/logger"
	"github.com/yashrajoria/api-gateway/utils"
	"go.uber.org/zap"
)

func RegisterAuthRoutes(r *gin.Engine) {
	authGroup := r.Group("/auth")

	// Forwarding logic for auth service
	authGroup.GET("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://auth-service:8081",
		})
	})
	authGroup.POST("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://auth-service:8081",
		})
	})
	authGroup.PUT("/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://auth-service:8081",
		})
	})

	logger.Log.Info("Auth routes registered", zap.String("group", "/auth"))
}
