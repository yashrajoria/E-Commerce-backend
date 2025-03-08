package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/routes"
)

func main() {
	// Connect to database
	if err := database.Connect(); err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	// Initialize Gin
	r := gin.Default()

	// Register product routes
	routes.RegisterProductRoutes(r)
	routes.RegisterCategoryRoutes(r)

	// Start server
	r.Run(":8082")
}
