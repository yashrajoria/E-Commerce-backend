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

	// Connect to database
	err := database.Connect()
	if err != nil {
		log.Println("Error connecting to database", zap.Error(err))
	}

	// Run migrations
	if err := database.DB.AutoMigrate(&models.Order{}, &models.OrderItem{}); err != nil {
		log.Println("Migration failed", zap.Error(err))
	}

	r := gin.Default()

	// Register order routes
	routes.RegisterOrderRoutes(r)

	// Start server on configured port
	if err := r.Run(":" + "8083"); err != nil {
		log.Println("Error starting server", zap.Error(err))
	}
}
