package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/inventory-service/controllers"
	db "github.com/yashrajoria/inventory-service/database"
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

	err = db.Connect()
	if err != nil {
		log.Println("Error connecting to database", zap.Error(err))
	}

	r := gin.Default()

	// Apply request logging
	//	r.Use(logger.RequestLogger())

	r.GET("/inventory/:productId", controllers.GetInventory)
	// r.POST("/inventory", controllers.AddInventory)
	// r.PUT("/inventory/:productId", controllers.UpdateInventory)

	//logger.Log.Info("Inventory Service started", zap.String("port", cfg.Port))
	// Start server on configured port
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Println("Error starting server", zap.Error(err))
	}
}
