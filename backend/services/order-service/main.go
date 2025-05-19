package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/order-service/database"
	"github.com/yashrajoria/order-service/models"
	"github.com/yashrajoria/order-service/routes"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	// 	logger.Initialize(os.Getenv("ENV"))

	// Load configuration from environment variables
	cfg, err := LoadConfig()
	if err != nil {
		log.Println("Config error", zap.Error(err))
	}

	// Connect to database
	err = database.Connect()
	if err != nil {
		log.Println("Error connecting to database", zap.Error(err))
	}

	// Run migrations
	if err := database.DB.AutoMigrate(&models.Order{}, &models.OrderItem{}); err != nil {
		log.Println("Migration failed", zap.Error(err))
	}

	r := gin.Default()

	// Apply request logging
	//	r.Use(logger.RequestLogger())

	// Register order routes
	routes.RegisterOrderRoutes(r)

	//logger.Log.Info("Order Service started", zap.String("port", cfg.Port))
	// Start server on configured port
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Println("Error starting server", zap.Error(err))
	}
}
