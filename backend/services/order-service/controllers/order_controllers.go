package controllers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	db "github.com/yashrajoria/order-service/database"
	models "github.com/yashrajoria/order-service/database"
	"go.mongodb.org/mongo-driver/bson"
)

func GetOrder(c *gin.Context) {

	collection := db.DB.Collection("orders")
	var orders []models.Order

	cursor, err := collection.Find(c, bson.M{})
	if err != nil {
		log.Println("Error finding products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch orders"})
		return
	}
	defer cursor.Close(c)

	// Decode each document and append it to the products slice
	for cursor.Next(c) {
		var order models.Order
		if err := cursor.Decode(&order); err != nil {
			log.Println("Error decoding order:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode order"})
			return
		}
		orders = append(orders, order)
	}

	// Handle cursor errors
	if err := cursor.Err(); err != nil {
		log.Println("Cursor error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cursor error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"products": orders})

}
