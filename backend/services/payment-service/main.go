package main

import (
	"log"
	"strings"

	"payment-service/config"
	"payment-service/controllers"
	"payment-service/database"
	"payment-service/kafka"
	"payment-service/routes"
	"payment-service/services"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("[PaymentService] ❌ Failed to load config:", err)
	}

	// Connect DB
	if err := database.Connect(); err != nil {
		log.Fatal("[PaymentService] ❌ Failed to connect to DB:", err)
	}

	log.Println(cfg)

	// Stripe + Kafka setup
	stripeSvc := services.NewStripeService(cfg.StripeSecretKey, cfg.StripeWebhookKey)
	groupID := "payment-service-group" // use a unique group name
	paymentProducer := kafka.NewPaymentEventProducer(strings.Split(cfg.KafkaBrokers, ","), cfg.KafkaTopic)
	paymentRequestConsumer := services.NewPaymentRequestConsumer(
		strings.Split(cfg.KafkaBrokers, ","),
		groupID,
		paymentProducer,
		stripeSvc,
	)
	// Start consuming payment requests in the background
	go paymentRequestConsumer.Start()

	defer paymentProducer.Close()

	// HTTP server
	r := gin.New()
	r.Use(gin.Recovery())

	pc := &controllers.PaymentController{
		Stripe: stripeSvc,
		Kafka:  paymentProducer,
	}
	routes.RegisterPaymentRoutes(r, pc)

	log.Println("[PaymentService] ✅ Running on port", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal("[PaymentService] ❌ Server failed:", err)
	}
}
