package main

import (
	"auth-service/controllers"
	"auth-service/database"
	"auth-service/models"
	"log"
	"net/http"

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
