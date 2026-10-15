// Package routes registers HTTP endpoints for payment processing and Stripe webhook callbacks.
package routes

import (
	"order-service/middleware"
	"order-service/payment/controllers"

	"github.com/gin-gonic/gin"
)

// RegisterPaymentRoutes configures and mounts HTTP route handlers on the router.
func RegisterPaymentRoutes(r *gin.Engine, pc *controllers.PaymentController) {
	// NOTE: /health* is owned by main.go — do not re-register here (gin panics
	// on duplicate route registration at boot).

	payments := r.Group("/payment")
	payments.Use(middleware.AuthMiddleware())
	{
		payments.GET("/status/by-order/:order_id", pc.GetPaymentStatusByOrderID)
		payments.POST("/create-checkout", pc.CreateCheckoutSession)
		payments.POST("/verify-payment", pc.VerifyPayment)
		payments.GET("/methods", pc.GetSavedPaymentMethods)
		payments.POST("/methods", pc.SavePaymentMethod)
		payments.DELETE("/methods/:id", pc.DeletePaymentMethod)
	}

	// Stripe webhook (no auth)
	r.POST("/stripe/webhook", pc.StripeWebhook)
}
