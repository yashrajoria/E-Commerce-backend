// // File: routes/auth.go
// package routes

// import (
// 	"github.com/gin-gonic/gin"
// 	"github.com/yashrajoria/api-gateway/logger"
// 	"github.com/yashrajoria/api-gateway/utils"
// 	"go.uber.org/zap"
// )

// func RegisterAuthRoutes(r *gin.Engine) {
// 	authGroup := r.Group("/auth")

// 	// Handle CORS preflight manually (optional, or use middleware globally)
// 	authGroup.OPTIONS("/*any", func(c *gin.Context) {
// 		origin := c.Request.Header.Get("Origin")
// 		if origin == "" {
// 			origin = "http://localhost:3000"
// 		}
// 		c.Header("Access-Control-Allow-Origin", origin)
// 		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
// 		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
// 		c.Header("Access-Control-Allow-Credentials", "true")
// 		c.Status(200)
// 	})

// 	// Forwarding logic
// 	authGroup.GET("/*any", func(c *gin.Context) {
// 		utils.ForwardRequest(c, utils.ForwardOptions{
// 			TargetBase: "http://auth-service:8081",
// 		})
// 	})
// 	authGroup.POST("/*any", func(c *gin.Context) {
// 		utils.ForwardRequest(c, utils.ForwardOptions{
// 			TargetBase: "http://auth-service:8081",
// 		})
// 	})
// 	authGroup.PUT("/*any", func(c *gin.Context) {
// 		utils.ForwardRequest(c, utils.ForwardOptions{
// 			TargetBase: "http://auth-service:8081",
// 		})
// 	})

// 	logger.Log.Info("Auth routes registered", zap.String("group", "/auth"))
// }

// File: routes/auth.go
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
