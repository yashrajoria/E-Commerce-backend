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
	var requestBody struct {
		Name        string   `json:"name"`
		ParentNames []string `json:"parent_names"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	var parentIDs []primitive.ObjectID
	var ancestorIDsSet = map[primitive.ObjectID]bool{}

	if len(requestBody.ParentNames) > 0 {
		cursor, err := database.DB.Collection("categories").Find(c, bson.M{
			"name": bson.M{"$in": requestBody.ParentNames},
		})
		if err != nil {
			log.Println("Error finding parent categories:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find parent categories"})
			return
		}
		defer cursor.Close(c)

		for cursor.Next(c) {
			var parent models.Category
			if err := cursor.Decode(&parent); err == nil {
				parentIDs = append(parentIDs, parent.ID)
				ancestorIDsSet[parent.ID] = true
				for _, ancestor := range parent.Ancestors {
					ancestorIDsSet[ancestor] = true
				}
			}
		}

		if len(parentIDs) != len(requestBody.ParentNames) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "One or more parent category names not found"})
			return
		}
	}

	// Convert map to slice
	var ancestorIDs []primitive.ObjectID
	for id := range ancestorIDsSet {
		ancestorIDs = append(ancestorIDs, id)
	}

	newCategory := models.Category{
		ID:        primitive.NewObjectID(),
		Name:      requestBody.Name,
		ParentIDs: parentIDs,
		Ancestors: ancestorIDs,
	}

	_, err := database.DB.Collection("categories").InsertOne(c, newCategory)
	if err != nil {
		log.Println("Error inserting category:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert category"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Category created successfully",
		"category": newCategory,
	})
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
