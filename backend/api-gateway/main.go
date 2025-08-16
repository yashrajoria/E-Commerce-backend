package main

import (
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/api-gateway/logger"
	"github.com/yashrajoria/api-gateway/routes"
	"go.uber.org/zap"
)

func main() {
	logger.InitLogger()
	defer logger.Sync()

	logger.Log.Info("Starting API Gateway...")

	r := gin.New()

	// Global middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	// CORS middleware
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Register routes from modular route packages
	routes.RegisterAllRoutes(r)

	// Read port from env or default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Log.Info("API Gateway listening on port", zap.String("port", port))

	if err := r.Run(":" + port); err != nil {
		logger.Log.Fatal("Failed to start server", zap.Error(err))
	}
}
