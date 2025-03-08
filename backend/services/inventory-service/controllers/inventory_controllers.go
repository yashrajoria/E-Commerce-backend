package controllers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	db "github.com/yashrajoria/inventory-service/database"
	models "github.com/yashrajoria/inventory-service/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func GetInventory(c *gin.Context) {
	if c.Param("productID") == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing product ID"})
		return
	}
	productID := c.Param("productID")

	objectId, err := primitive.ObjectIDFromHex(productID)

	if err != nil {
		log.Println("Invalid product ID format:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch inventory for product"})

		return
	}

	var inventory []models.Inventory

	err = db.DB.Collection("products").FindOne(c, bson.M{"_id": objectId}).Decode(&inventory)
	if err != nil {
		log.Println("Error finding product:", err)
		c.JSON(http.StatusNotFound, gin.H{"message": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, inventory)

}
