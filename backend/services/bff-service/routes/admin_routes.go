package routes

import (
	"bff-service/controllers"
	"bff-service/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterAdminRoutes(
	r *gin.Engine,
	logger *zap.Logger,
	productCtrl *controllers.AdminProductController,
	categoryCtrl *controllers.AdminCategoryController,
	userCtrl *controllers.AdminUserController,
	orderCtrl *controllers.AdminOrderController,
	inventoryCtrl *controllers.AdminInventoryController,
	promotionCtrl *controllers.AdminPromotionController,
	notificationCtrl *controllers.AdminNotificationController,
	analyticsCtrl *controllers.AdminAnalyticsController,
	dashboardCtrl *controllers.AdminDashboardController,
) {
	admin := r.Group("/bff/admin")
	admin.Use(middleware.RequestLoggerMiddleware(logger))
	admin.Use(middleware.AdminAuthMiddleware())

	if dashboardCtrl != nil {
		admin.GET("/dashboard", dashboardCtrl.GetDashboardSummary)
	}

	if productCtrl != nil {
		admin.GET("/products", productCtrl.ListProducts)
		admin.POST("/products", productCtrl.CreateProduct)
		admin.GET("/products/presign", productCtrl.GetPresignUpload)
		admin.PUT("/products/:id", productCtrl.UpdateProduct)
		admin.POST("/products/:id/images/presign", productCtrl.PostProductImagePresign)
		admin.DELETE("/products/:id", productCtrl.DeleteProduct)
	}

	if categoryCtrl != nil {
		admin.GET("/categories", categoryCtrl.ListCategories)
		admin.POST("/categories", categoryCtrl.CreateCategory)
		admin.PUT("/categories/:id", categoryCtrl.UpdateCategory)
		admin.DELETE("/categories/:id", categoryCtrl.DeleteCategory)
	}

	if userCtrl != nil {
		admin.GET("/users", userCtrl.ListUsers)
		admin.PUT("/users/:id/role", userCtrl.UpdateUserRole)
		admin.DELETE("/users/:id", userCtrl.DeleteUser)
	}

	if orderCtrl != nil {
		admin.GET("/orders", orderCtrl.ListOrders)
		admin.GET("/orders/:id", orderCtrl.GetOrderByID)
		admin.PUT("/orders/:id/status", orderCtrl.UpdateOrderStatus)
	}

	if inventoryCtrl != nil {
		admin.GET("/inventory", inventoryCtrl.ListInventory)
		admin.PUT("/inventory/:product_id", inventoryCtrl.UpdateInventory)
	}

	if promotionCtrl != nil {
		admin.GET("/coupons", promotionCtrl.ListCoupons)
		admin.POST("/coupons", promotionCtrl.CreateCoupon)
		admin.PUT("/coupons/:id", promotionCtrl.UpdateCoupon)
		admin.DELETE("/coupons/:id", promotionCtrl.DeleteCoupon)
	}

	if notificationCtrl != nil {
		admin.GET("/notifications", notificationCtrl.ListNotifications)
		admin.GET("/notifications/log", notificationCtrl.NotificationLog)
	}

	if analyticsCtrl != nil {
		admin.GET("/reports/sales", analyticsCtrl.SalesReport)
		admin.GET("/reports/users", analyticsCtrl.UsersReport)
		admin.GET("/reports/inventory", analyticsCtrl.InventoryReport)
	}
}
