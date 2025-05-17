package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/order-service/database"
	"github.com/yashrajoria/order-service/models"
	"github.com/yashrajoria/order-service/routes"
)

func main() {
	err := database.Connect()
	database.DB.AutoMigrate(&models.Order{}, &models.OrderItem{})

	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	r := gin.Default()

	// Register order routes
	routes.RegisterOrderRoutes(r)
	r.Run(":8083")

}
