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
