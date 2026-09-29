package routes

import (
	"order-service/payment/controllers"
	"order-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterPaymentRoutes(r *gin.Engine, pc *controllers.PaymentController) {
	live := func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "order-service"})
	}
	r.GET("/health", live)
	r.GET("/health/live", live)
	r.GET("/health/ready", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ready", "service": "order-service"})
	})

	payments := r.Group("/payment")
	payments.Use(middleware.AuthMiddleware())
	{
		payments.GET("/status/by-order/:order_id", pc.GetPaymentStatusByOrderID)
		payments.POST("/create-checkout", pc.CreateCheckoutSession)
		payments.POST("/verify-payment", pc.VerifyPayment)
	}

	// Stripe webhook (no auth)
	r.POST("/stripe/webhook", pc.StripeWebhook)
}
