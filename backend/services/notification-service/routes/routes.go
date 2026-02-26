package routes

import (
	"net/http"
	"notification-service/controllers"
	"notification-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, controller *controllers.NotificationController) {
	// Public
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK", "service": "notification-service"})
	})

	// Admin only
	admin := router.Group("/notifications", middleware.AuthMiddleware(), middleware.AdminOnly())
	{
		admin.GET("/log", controller.GetNotificationLogs)
	}
}
