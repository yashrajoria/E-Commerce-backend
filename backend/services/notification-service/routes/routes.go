package routes

import (
	"net/http"
	"notification-service/controllers"
	"notification-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.Engine, controller *controllers.NotificationController) {
	// Public
	live := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "notification-service"}) }
	router.GET("/health", live)
	router.GET("/health/live", live)
	router.GET("/health/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "service": "notification-service"})
	})

	// Admin only
	admin := router.Group("/notifications", middleware.AuthMiddleware(), middleware.AdminOnly())
	{
		admin.GET("/log", controller.GetNotificationLogs)
	}
}
