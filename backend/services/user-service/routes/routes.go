package routes

import (
	"user-service/controllers"

	"github.com/gin-gonic/gin"
	commonlog "github.com/yashrajoria/common/logger"
)

// Accepts a RouterGroup which already applies auth middleware
func RegisterUserRoutes(rg *gin.RouterGroup, ctrl *controllers.UserController) {
	commonlog.Log.Info("Registering user routes")
	rg.GET("/profile", ctrl.GetProfile)
	rg.PUT("/profile", ctrl.UpdateProfile)
	rg.POST("/change-password", ctrl.ChangePassword)
}

// RegisterAdminRoutes registers admin-only user routes (list all users).
// The provided RouterGroup should already have AuthMiddleware + AdminOnly applied.
func RegisterAdminRoutes(rg *gin.RouterGroup, ctrl *controllers.UserController) {
	commonlog.Log.Info("Registering admin user routes")
	rg.GET("", ctrl.GetAllUsers)
}
