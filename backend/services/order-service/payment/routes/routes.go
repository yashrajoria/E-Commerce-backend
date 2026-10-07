package routes

import (
	"order-service/payment/controllers"
	"order-service/middleware"

	"github.com/gin-gonic/gin"
)

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
