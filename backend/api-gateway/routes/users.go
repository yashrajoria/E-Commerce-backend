package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/middlewares"
	"github.com/yashrajoria/api-gateway/utils"
)

func RegisterUserRoutes(r *gin.RouterGroup) {
	userGroup := r.Group("/")
	userGroup.Use(middlewares.JWTMiddleware())

	userGroup.GET("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})
	userGroup.PUT("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})
	userGroup.POST("/users/*any", func(c *gin.Context) {
		utils.ForwardRequest(c, utils.ForwardOptions{
			TargetBase: "http://user-service:8085/users",
		})
	})

}
