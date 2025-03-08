package controllers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func CreateCategory(c *gin.Context) {
	var category models.Category

	if err := c.ShouldBindJSON(&category); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	if category.ParentID != nil && category.ParentID.Hex() == "" {
		category.ParentID = nil
	}

	category.ID = primitive.NewObjectID()

	_, err := database.DB.Collection("categories").InsertOne(c, category)

	if err != nil {
		log.Println("Error inserting category:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert category"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Category created successfully", "category": category})

}

func GetCategories(c *gin.Context) {

	collection := database.DB.Collection("categories")

	var categories []models.Category

	cursor, err := collection.Find(c, bson.M{})

	if err != nil {
		log.Println("Error finding categories:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}

	defer cursor.Close(c)

	for cursor.Next(c) {
		var category models.Category
		if err := cursor.Decode(&category); err != nil {
			log.Println("Error decoding category:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode category"})
			return
		}
		categories = append(categories, category)
	}

	if err := cursor.Err(); err != nil {
		log.Println("Cursor error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cursor error"})
		return
	}

	c.JSON(http.StatusOK, categories)

}
