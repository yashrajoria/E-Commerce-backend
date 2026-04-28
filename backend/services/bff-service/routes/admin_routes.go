package routes

import (
	"bff-service/controllers"
	"bff-service/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RegisterAdminRoutes registers the BFF admin aggregation endpoints.
//
// IMPORTANT — Route ownership:
// The api-gateway routes most /bff/admin/* requests directly to the relevant
// microservices (product-service, order-service, etc.) to avoid a double-proxy
// hop through the BFF. The BFF only serves endpoints that require cross-service
// aggregation OR that the api-gateway explicitly forwards here:
//
//   - GET  /bff/admin/dashboard            — multi-service KPI aggregation
//   - GET  /bff/admin/reports/*            — analytics forwarded to downstream
//   - GET  /bff/admin/products/presign     — presigned upload URL (proxied to product-service)
//   - POST /bff/admin/products/:id/images/presign — image presign (proxied to product-service)
func RegisterAdminRoutes(
	r *gin.Engine,
	logger *zap.Logger,
	productCtrl *controllers.AdminProductController,
	analyticsCtrl *controllers.AdminAnalyticsController,
	dashboardCtrl *controllers.AdminDashboardController,
) {
	admin := r.Group("/bff/admin")
	admin.Use(middleware.RequestLoggerMiddleware(logger))
	admin.Use(middleware.AdminAuthMiddleware())

	if dashboardCtrl != nil {
		admin.GET("/dashboard", dashboardCtrl.GetDashboardSummary)
	}

	if analyticsCtrl != nil {
		admin.GET("/reports/sales", analyticsCtrl.SalesReport)
		admin.GET("/reports/users", analyticsCtrl.UsersReport)
		admin.GET("/reports/inventory", analyticsCtrl.InventoryReport)
	}

	// Presign routes: the api-gateway forwards these to the BFF so the BFF can
	// add auth headers before proxying on to product-service.
	if productCtrl != nil {
		admin.GET("/products/presign", productCtrl.GetPresignUpload)
		admin.POST("/products/:id/images/presign", productCtrl.PostProductImagePresign)
	}
}
