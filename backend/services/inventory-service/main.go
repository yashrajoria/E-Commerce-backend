package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/inventory-service/controllers"
	db "github.com/yashrajoria/inventory-service/database"
)

func main() {
	err := db.Connect()

	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	r := gin.Default()

	r.GET("/inventory/:productId", controllers.GetInventory)
	// r.POST("/inventory", controllers.AddInventory)
	// r.PUT("/inventory/:productId", controllers.UpdateInventory)

	r.Run(":8084")

}
