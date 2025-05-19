package main

import (
	"auth-service/controllers"
	"auth-service/database"
	middlewares "auth-service/middleware"
	"auth-service/models"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize logger
	// 	logger.Initialize(os.Getenv("ENV"))

	// Load configuration from environment variables
	cfg, err := LoadConfig()
	if err != nil {
		log.Println("Config error", err)
	}

	// Connect to the database
	if err := database.Connect(); err != nil {
		log.Println("Could not connect to PostgreSQL", err)
		return
	}

	// Run migrations
	if err := models.Migrate(database.DB); err != nil {
		log.Println("Migration failed", err)
	}

	// Initialize Gin router
	r := gin.Default()

	// Apply security headers to all routes
	r.Use(middlewares.SecurityHeaders())

	// Apply rate limiting to all routes
	r.Use(middlewares.RateLimitMiddleware())

	// Apply request logging
	//	r.Use(logger.RequestLogger())

	// CORS configuration
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "http://localhost:3000" // Default origin
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})

	// Health check route
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	// Auth routes
	authGroup := r.Group("/auth")
	{
		// Public routes
		authGroup.POST("/register", controllers.Register)
		authGroup.POST("/login", controllers.Login)
		authGroup.POST("/verify-email", controllers.VerifyEmail)

		// Protected routes
		protected := authGroup.Group("")
		protected.Use(middlewares.RefreshTokenMiddleware())
		{
			protected.POST("/address", controllers.CreateAddress)
			// Add more protected routes here
		}
	}

	//logger.Log.Info("Auth Service started", "port", cfg.Port)
	// Start the server on configured port
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Println("Error starting server", err)
	}
}
