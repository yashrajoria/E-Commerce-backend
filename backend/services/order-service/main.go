package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/order-service/controllers"
	db "github.com/yashrajoria/order-service/database"
)

func main() {
	err := db.Connect()

	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	r := gin.Default()

	r.GET("/orders", controllers.GetOrder)

	r.Run(":8083")

}
