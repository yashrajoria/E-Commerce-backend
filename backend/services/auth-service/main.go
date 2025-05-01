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

		return
	}

	// Run migrations
	if err := models.Migrate(database.DB); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	// Initialize Gin router
	r := gin.Default()
	authGroup := r.Group("/auth")
	authGroup.OPTIONS("/*any", func(c *gin.Context) {
		log.Println("Handling OPTIONS request for:", c.Request.URL.Path)
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			origin = "http://localhost:3000" // Default origin
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Status(http.StatusOK)
	})

	// Health check route
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	// Auth routes
	authGroup.POST("/register", controllers.Register)
	authGroup.POST("/login", controllers.Login)
	authGroup.POST("/verify-email", controllers.VerifyEmail)
	authGroup.POST("/address", controllers.CreateAddress)

	//Address Routes
	// routes.RegisterAddressRoutes(r)

	log.Println("Auth Service started on port 8081")
	// Start the server on port 8081
	if err := r.Run(":8081"); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
