package admin

import (
	"order-service/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterDashboardRoutes mounts the admin dashboard aggregation absorbed
// from bff-service. Route is admin-gated like the rest of /orders/admin.
func RegisterDashboardRoutes(r *gin.Engine, ctrl *DashboardController) {
	r.GET("/orders/admin/dashboard", middleware.AuthMiddleware(), middleware.AdminOnly(), ctrl.GetDashboardSummary)
}
