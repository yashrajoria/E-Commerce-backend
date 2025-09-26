package controllers

import (
	"encoding/csv"

	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type CategoryInput struct {
	Name        string   `json:"name" binding:"required"`
	ParentNames []string `json:"parent_names,omitempty"`
	Image       string   `json:"image,omitempty"`
	IsActive    bool     `json:"is_active,omitempty"`
}

// Shared helper for single + bulk
func processCategoryInput(c *gin.Context, input CategoryInput) (*models.Category, error) {
	collection := database.DB.Collection("categories")

	// 1️⃣ Check duplicates
	var existing models.Category
	err := collection.FindOne(c, bson.M{"name": input.Name}).Decode(&existing)
	if err == nil {
		// already exists
		return nil, nil
	} else if err != mongo.ErrNoDocuments {
		return nil, err
	}

	// 2️⃣ Resolve parents
	var parentIDs []uuid.UUID
	ancestorSet := map[uuid.UUID]bool{}
	if len(input.ParentNames) > 0 {
		cursor, err := collection.Find(c, bson.M{"name": bson.M{"$in": input.ParentNames}})
		if err != nil {
			return nil, err
		}
		defer cursor.Close(c)

		foundParents := map[string]bool{}
		for cursor.Next(c) {
			var parent models.Category
			if err := cursor.Decode(&parent); err != nil {
				continue
			}
			parentIDs = append(parentIDs, parent.ID)
			ancestorSet[parent.ID] = true
			for _, anc := range parent.Ancestors {
				ancestorSet[anc] = true
			}
			foundParents[parent.Name] = true
		}

		for _, parentName := range input.ParentNames {
			if !foundParents[parentName] {
				return nil, mongo.ErrNoDocuments
			}
		}
	}

	ancestorIDs := make([]uuid.UUID, 0, len(ancestorSet))
	for id := range ancestorSet {
		ancestorIDs = append(ancestorIDs, id)
	}

	now := time.Now().UTC()
	slug := strings.ToLower(input.Name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")

	newCategory := models.Category{
		ID:        uuid.New(),
		Name:      input.Name,
		ParentIDs: parentIDs,
		Ancestors: ancestorIDs,
		Image:     input.Image,
		Slug:      slug,
		IsActive:  input.IsActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = collection.InsertOne(c, newCategory)
	if err != nil {
		return nil, err
	}

	return &newCategory, nil
}

// 📌 Create single category
func CreateCategory(c *gin.Context) {
	var input CategoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	category, err := processCategoryInput(c, input)
	if err == mongo.ErrNoDocuments {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Parent category not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if category == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Category with this name already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Category created successfully",
		"category": category,
	})
}

// 📌 Bulk create categories (JSON or CSV)
func BulkCreateCategories(c *gin.Context) {
	var inputs []CategoryInput

	// 1️⃣ If JSON
	if c.ContentType() == "application/json" {
		if err := c.ShouldBindJSON(&inputs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
			return
		}
	} else {
		// 2️⃣ If CSV
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "CSV file is required"})
			return
		}
		f, _ := file.Open()
		defer f.Close()

		reader := csv.NewReader(f)
		records, err := reader.ReadAll()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read CSV"})
			return
		}

		for i, record := range records {
			if i == 0 {
				continue
			}
			isActive := true
			if len(record) > 3 && strings.ToLower(record[3]) == "false" {
				isActive = false
			}
			parentNames := []string{}
			if len(record) > 1 && record[1] != "" {
				parentNames = strings.Split(record[1], ";")
			}
			inputs = append(inputs, CategoryInput{
				Name:        record[0],
				ParentNames: parentNames,
				Image:       record[2],
				IsActive:    isActive,
			})
		}
	}

	inserted := []string{}
	skipped := []string{}

	for _, input := range inputs {
		category, err := processCategoryInput(c, input)
		if err == mongo.ErrNoDocuments {
			skipped = append(skipped, input.Name+" (parent not found)")
			continue
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if category == nil {
			skipped = append(skipped, input.Name+" (duplicate)")
			continue
		}
		inserted = append(inserted, input.Name)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Bulk categories processed",
		"inserted": inserted,
		"skipped":  skipped,
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

	// Build a map of category ID -> category
	categoryMap := make(map[uuid.UUID]*models.Category)
	for i := range categories {
		categoryMap[categories[i].ID] = &categories[i]
	}

	// Build the tree
	var rootCategories []*models.Category
	for i := range categories {
		cat := &categories[i]
		if len(cat.ParentIDs) == 0 {
			// No parent → root category
			rootCategories = append(rootCategories, cat)
		} else {
			// Attach to parent(s)
			for _, parentID := range cat.ParentIDs {
				if parent, ok := categoryMap[parentID]; ok {
					// Use a Children field in the model for nested categories
					parent.Children = append(parent.Children, cat)
				}
			}
		}
	}

	c.JSON(http.StatusOK, rootCategories)
}

func GetCategoryByID(c *gin.Context) {
	id := c.Param("id")
	objectID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	filter := bson.M{"_id": objectID, "deleted_at": bson.M{"$exists": false}}

	var cat models.Category
	err = database.DB.Collection("categories").FindOne(c, filter).Decode(&cat)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Fetch all categories for mapping parent names and path
	cursor, err := database.DB.Collection("categories").Find(c, bson.M{"deleted_at": bson.M{"$exists": false}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}
	defer cursor.Close(c)

	var allCategories []models.Category
	if err := cursor.All(c, &allCategories); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode categories"})
		return
	}

	categoryMap := make(map[uuid.UUID]models.Category)
	for _, c := range allCategories {
		categoryMap[c.ID] = c
	}

	var parentNames []string
	var path []string
	currentIDs := cat.ParentIDs
	for len(currentIDs) > 0 {
		parentID := currentIDs[0]
		if parent, ok := categoryMap[parentID]; ok {
			parentNames = append(parentNames, parent.Name)
			path = append([]string{parent.Name}, path...)
			currentIDs = parent.ParentIDs
		} else {
			break
		}
	}
	path = append(path, cat.Name)

	type EnhancedCategory struct {
		models.Category
		ParentNames []string `json:"parent_names,omitempty"`
		Path        []string `json:"path,omitempty"`
		Level       int      `json:"level,omitempty"`
	}

	enhancedCategory := EnhancedCategory{
		Category:    cat,
		ParentNames: parentNames,
		Path:        path,
		Level:       len(path) - 1,
	}

	c.JSON(http.StatusOK, enhancedCategory)
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
