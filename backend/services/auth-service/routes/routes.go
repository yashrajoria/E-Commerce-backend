package routes

import (
	"auth-service/controllers"

	"github.com/gin-gonic/gin"
)

func RegisterAuthRoutes(r *gin.Engine, authController *controllers.AuthController) {
	authRoutes := r.Group("/auth")
	{
		authRoutes.POST("/register", authController.Register)
		authRoutes.POST("/login", authController.Login)
		authRoutes.POST("/verify-email", authController.VerifyEmail)
		authRoutes.POST("/logout", authController.Logout)
		authRoutes.GET("/status", authController.GetAuthStatus)
		authRoutes.POST("/refresh", authController.Refresh)

	}
}
