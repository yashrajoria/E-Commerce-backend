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
		Name        string   `json:"name" binding:"required"`
		ParentNames []string `json:"parent_names,omitempty"`
		Image       string   `json:"image,omitempty"`
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
	ancestorIDsSet := map[uuid.UUID]bool{}

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

	ancestorIDs := make([]uuid.UUID, 0, len(ancestorIDsSet))
	for id := range ancestorIDsSet {
		ancestorIDs = append(ancestorIDs, id)
	}

	now := time.Now().UTC()
	newCategory := models.Category{
		ID:        uuid.New(),
		Name:      requestBody.Name,
		ParentIDs: parentIDs,
		Ancestors: ancestorIDs,
		Image:     requestBody.Image,
		CreatedAt: now,
		UpdatedAt: now,
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

	filter := bson.M{"deleted_at": bson.M{"$exists": false}}

	cursor, err := collection.Find(c, filter)
	if err != nil {
		log.Println("Error finding categories:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}
	defer cursor.Close(c)

	var categories []models.Category
	if err := cursor.All(c, &categories); err != nil {
		log.Println("Error decoding categories:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode categories"})
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

	filter := bson.M{"_id": objectID, "deleted_at": bson.M{"$exists": false}}

	var category models.Category
	err = database.DB.Collection("categories").FindOne(c, filter).Decode(&category)
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
		ParentNames []string `json:"parent_names,omitempty"`
		Image       string   `json:"image,omitempty"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	update := bson.M{
		"$set": bson.M{
			"name":       requestBody.Name,
			"image":      requestBody.Image,
			"updated_at": time.Now().UTC(),
		},
	}

	if len(requestBody.ParentNames) > 0 {
		var parentIDs []uuid.UUID
		ancestorIDsSet := map[uuid.UUID]bool{}

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

		ancestorIDs := make([]uuid.UUID, 0, len(ancestorIDsSet))
		for id := range ancestorIDsSet {
			ancestorIDs = append(ancestorIDs, id)
		}

		update["$set"].(bson.M)["parent_ids"] = parentIDs
		update["$set"].(bson.M)["ancestors"] = ancestorIDs
	}

	result, err := database.DB.Collection("categories").UpdateOne(c,
		bson.M{"_id": objectID, "deleted_at": bson.M{"$exists": false}},
		update,
	)
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

	count, err := database.DB.Collection("products").CountDocuments(c, bson.M{
		"category_ids": objectID,
		"deleted_at":   bson.M{"$exists": false},
	})
	if err != nil {
		log.Println("Error checking products in category:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check category products"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot delete category with associated products"})
		return
	}

	result, err := database.DB.Collection("categories").UpdateOne(
		c,
		bson.M{"_id": objectID, "deleted_at": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"deleted_at": time.Now().UTC()}},
	)
	if err != nil {
		log.Println("Error deleting category:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete category"})
		return
	}
	if result.ModifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found or already deleted"})
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

	filter := bson.M{
		"category_ids": objectID,
		"deleted_at":   bson.M{"$exists": false},
	}

	cursor, err := database.DB.Collection("products").Find(c, filter)
	if err != nil {
		log.Println("Error finding products for category:", err)
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
