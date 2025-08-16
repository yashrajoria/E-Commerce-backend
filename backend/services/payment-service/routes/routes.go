package routes

import (
    "payment-service/controllers"
    "payment-service/middleware"

    "github.com/gin-gonic/gin"
)

func RegisterPaymentRoutes(r *gin.Engine, pc *controllers.PaymentController) {
    payments := r.Group("/payments")
    payments.Use(middleware.AuthMiddleware())
    payments.POST("/initiate", pc.InitiatePayment)

    // Stripe webhook (no auth)
    r.POST("/stripe/webhook", pc.StripeWebhook)
}
