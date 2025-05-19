package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/routes"
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

	// Connect to database using config values
	if err := database.ConnectWithConfig(cfg.MongoURL, cfg.Database); err != nil {
		log.Println("Error connecting to database", zap.Error(err))
	}

	// Initialize Gin
	r := gin.Default()

	// Apply request logging
	//	r.Use(logger.RequestLogger())

	// Register product routes
	routes.RegisterProductRoutes(r)
	routes.RegisterCategoryRoutes(r)

	//logger.Log.Info("Product Service started", zap.String("port", cfg.Port))
	// Start server on configured port
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Println("Error starting server", zap.Error(err))
	}
}
