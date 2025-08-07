package controllers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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

	var existingCategory models.Category
	err := database.DB.Collection("categories").FindOne(c, bson.M{"name": requestBody.Name}).Decode(&existingCategory)

	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Category with this name already exists"})
		return
	} else if err != mongo.ErrNoDocuments {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	var parentIDs []uuid.UUID
	var ancestorIDsSet = map[uuid.UUID]bool{}

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
	var ancestorIDs []uuid.UUID
	for id := range ancestorIDsSet {
		ancestorIDs = append(ancestorIDs, id)
	}

	newCategory := models.Category{
		ID:        uuid.New(),
		Name:      requestBody.Name,
		ParentIDs: parentIDs,
		Ancestors: ancestorIDs,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = database.DB.Collection("categories").InsertOne(c, newCategory)
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

func GetCategoryByID(c *gin.Context) {
	id := c.Param("id")
	objectID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	var category models.Category
	err = database.DB.Collection("categories").FindOne(c, bson.M{"_id": objectID}).Decode(&category)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	c.JSON(http.StatusOK, category)
}

func UpdateCategory(c *gin.Context) {
	id := c.Param("id")
	objectID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	var requestBody struct {
		Name        string   `json:"name"`
		ParentNames []string `json:"parent_names"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	update := bson.M{
		"$set": bson.M{
			"name":       requestBody.Name,
			"updated_at": time.Now(),
		},
	}

	// If parent names are provided, update parent relationships
	if len(requestBody.ParentNames) > 0 {
		var parentIDs []uuid.UUID
		var ancestorIDsSet = map[uuid.UUID]bool{}

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

		// Convert map to slice
		var ancestorIDs []uuid.UUID
		for id := range ancestorIDsSet {
			ancestorIDs = append(ancestorIDs, id)
		}

		update["$set"].(bson.M)["parent_ids"] = parentIDs
		update["$set"].(bson.M)["ancestors"] = ancestorIDs
	}

	result, err := database.DB.Collection("categories").UpdateOne(c, bson.M{"_id": objectID}, update)
	if err != nil {
		log.Println("Error updating category:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update category"})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category updated successfully"})
}

func DeleteCategory(c *gin.Context) {
	id := c.Param("id")
	objectID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	// Check if category has any products
	count, err := database.DB.Collection("products").CountDocuments(c, bson.M{"category_id": objectID})
	if err != nil {
		log.Println("Error checking products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check category products"})
		return
	}

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete category with associated products"})
		return
	}

	result, err := database.DB.Collection("categories").DeleteOne(c, bson.M{"_id": objectID})
	if err != nil {
		log.Println("Error deleting category:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete category"})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}

func GetCategoryProducts(c *gin.Context) {
	id := c.Param("id")
	objectID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	cursor, err := database.DB.Collection("products").Find(c, bson.M{"category_id": objectID})
	if err != nil {
		log.Println("Error finding products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)

	var products []models.Product
	if err := cursor.All(c, &products); err != nil {
		log.Println("Error decoding products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	c.JSON(http.StatusOK, products)
}
