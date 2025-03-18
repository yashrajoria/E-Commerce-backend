package main

import (
	"auth-service/controllers"
	"auth-service/database"
	"auth-service/models"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"

	"github.com/gin-gonic/gin"
)

func main() {
	// Connect to the database
	if err := database.Connect(); err != nil {
		log.Fatalf("Could not connect to PostgreSQL: %v", err)
	}

	// Run migrations
	if err := models.Migrate(database.DB); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Initialize Gin router
	r := gin.Default()
	// CORS Configuration
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"}, // Allowed origins
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour, // Cache preflight request for 12 hours
	}))
	// Health check route
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	// Auth routes
	r.POST("/auth/register", controllers.Register) // Register (Signup)
	r.POST("/auth/login", controllers.Login)       // Login

	log.Println("Auth Service started on port 8081")
	// Start the server on port 8081
	if err := r.Run(":8081"); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
