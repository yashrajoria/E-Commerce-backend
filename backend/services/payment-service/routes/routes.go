package routes

import (
	"payment-service/controllers"
	"payment-service/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterPaymentRoutes(r *gin.Engine, pc *controllers.PaymentController) {
	live := func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "payment-service"})
	}
	r.GET("/health", live)
	r.GET("/health/live", live)
	r.GET("/health/ready", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ready", "service": "payment-service"})
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
