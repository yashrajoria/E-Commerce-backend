package routes

import (
    "log"
    "user-service/controllers"
    "github.com/gin-gonic/gin"
)

// Accepts a RouterGroup which already applies auth middleware
func RegisterUserRoutes(rg *gin.RouterGroup) {
    log.Println("Registering user routes...")
    rg.GET("/profile", controllers.GetProfile)
    rg.PUT("/profile", controllers.UpdateProfile)
    rg.POST("/change-password", controllers.ChangePassword)
}

// RegisterAdminRoutes registers admin-only user routes (list all users).
// The provided RouterGroup should already have AuthMiddleware + AdminOnly applied.
func RegisterAdminRoutes(rg *gin.RouterGroup) {
    log.Println("Registering admin user routes...")
    rg.GET("", controllers.GetAllUsers)
}
