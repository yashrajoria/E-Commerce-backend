package controllers

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetProducts retrieves paginated products from the database.
func GetProducts(c *gin.Context) {
	collection := database.DB.Collection("products")

	// Parse query parameters
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	perPage, err := strconv.Atoi(c.DefaultQuery("perPage", "10"))
	if err != nil || perPage <= 0 {
		perPage = 10
	}

	skip := (page - 1) * perPage

	// MongoDB query options
	findOptions := options.Find()
	findOptions.SetLimit(int64(perPage))
	findOptions.SetSkip(int64(skip))

	var products []models.Product
	cursor, err := collection.Find(c, bson.M{}, findOptions)
	if err != nil {
		log.Println("Error finding products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)

	if err := cursor.All(c, &products); err != nil {
		log.Println("Error decoding products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	total, err := collection.CountDocuments(c, bson.M{})
	if err != nil {
		log.Println("Error counting products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	// Respond with products and pagination metadata
	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"meta": gin.H{
			"page":       page,
			"perPage":    perPage,
			"total":      total,
			"totalPages": totalPages,
		},
	})
}

// GetProductByID retrieves a single product by ID.
func GetProductByID(c *gin.Context) {
	id := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid product ID format:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID format"})
		return
	}

	var product models.Product
	err = database.DB.Collection("products").FindOne(c, bson.M{"_id": objectID}).Decode(&product)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"message": "Product not found"})
		} else {
			log.Println("Database error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		}
		return
	}

	c.JSON(http.StatusOK, product)
}

type ProductInput struct {
	Name        string   `json:"name" binding:"required"`
	Price       float64  `json:"price" binding:"required"`
	Categories  []string `json:"category" binding:"required"`
	Images      []string `json:"images"`
	Quantity    int      `json:"quantity"`
	Description string   `json:"description"`
}

func CreateProduct(c *gin.Context) {
	var input ProductInput

	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("Invalid JSON body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	if len(input.Categories) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one category is required"})
		return
	}

	ctx := c.Request.Context()
	var categoryIDs []primitive.ObjectID
	var categoryPaths []string
	categorySet := make(map[primitive.ObjectID]bool)
	pathSet := make(map[string]bool)

	for _, catName := range input.Categories {
		var category models.Category
		err := database.DB.Collection("categories").FindOne(ctx, bson.M{"name": catName}).Decode(&category)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Category '%s' not found", catName)})
			return
		}

		if !categorySet[category.ID] {
			categoryIDs = append(categoryIDs, category.ID)
			categorySet[category.ID] = true
		}
		for _, ancestor := range category.Ancestors {
			if !categorySet[ancestor] {
				categoryIDs = append(categoryIDs, ancestor)
				categorySet[ancestor] = true
			}
		}

		for _, path := range category.Path {
			if !pathSet[path] {
				categoryPaths = append(categoryPaths, path)
				pathSet[path] = true
			}
		}
	}

	product := models.Product{
		ID:           primitive.NewObjectID(),
		Name:         input.Name,
		Price:        input.Price,
		Quantity:     input.Quantity,
		Description:  input.Description,
		Images:       input.Images,
		CategoryID:   categoryIDs[0],
		CategoryIDs:  categoryIDs,
		CategoryPath: categoryPaths,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err := database.DB.Collection("products").InsertOne(ctx, product)
	if err != nil {
		log.Println("Error inserting product:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert product"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Product created successfully", "product": product})
}

// DeleteProduct removes a product by ID.
func DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid product ID format:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID format"})
		return
	}

	result, err := database.DB.Collection("products").DeleteOne(c, bson.M{"_id": objectID})
	if err != nil {
		log.Println("Error deleting product:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}

// UpdateProduct updates an existing product.
func UpdateProduct(c *gin.Context) {
	id := c.Param("id")
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Println("Invalid product ID format:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid product ID format"})
		return
	}

	var updates bson.M
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No update fields provided"})
		return
	}

	result, err := database.DB.Collection("products").UpdateOne(c, bson.M{"_id": objectID}, bson.M{"$set": updates})
	if err != nil {
		log.Println("Error updating product:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}

	if result.ModifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or no changes made"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product updated successfully"})
}

func CreateBulkProducts(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		log.Println("Error getting file:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		log.Println("Error opening file:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	r := csv.NewReader(src)
	records, err := r.ReadAll()
	if err != nil {
		log.Println("Error reading CSV:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read CSV"})
		return
	}

	if len(records) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No product records in CSV"})
		return
	}
	records = records[1:] // Skip header

	// category name -> category object cache
	categoryCache := make(map[string]models.Category)
	var products []models.Product

	for _, row := range records {
		if len(row) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Each row must include: Name, Price, Categories, Description, Quantity, ImageURL"})
			return
		}

		name := strings.TrimSpace(row[0])
		price, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid price format"})
			return
		}

		rawCategories := strings.Split(row[2], ",")
		description := strings.TrimSpace(row[3])
		quantity, err := strconv.Atoi(strings.TrimSpace(row[4]))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quantity format"})
			return
		}
		image := strings.TrimSpace(row[5])

		var categoryIDs []primitive.ObjectID
		var categoryPaths []string
		categorySet := make(map[primitive.ObjectID]bool) // to dedupe ancestors

		for _, catNameRaw := range rawCategories {
			catName := strings.TrimSpace(catNameRaw)
			if catName == "" {
				continue
			}

			var cat models.Category
			if cached, ok := categoryCache[catName]; ok {
				cat = cached
			} else {
				err := database.DB.Collection("categories").FindOne(c, bson.M{"name": catName}).Decode(&cat)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Category '%s' not found", catName)})
					return
				}
				categoryCache[catName] = cat
			}

			if !categorySet[cat.ID] {
				categoryIDs = append(categoryIDs, cat.ID)
				categorySet[cat.ID] = true
			}

			for _, ancestor := range cat.Ancestors {
				if !categorySet[ancestor] {
					categoryIDs = append(categoryIDs, ancestor)
					categorySet[ancestor] = true
				}
			}

			categoryPaths = append(categoryPaths, cat.Path...)
		}

		// Deduplicate path strings
		pathSet := make(map[string]bool)
		var dedupedPaths []string
		for _, p := range categoryPaths {
			if !pathSet[p] {
				dedupedPaths = append(dedupedPaths, p)
				pathSet[p] = true
			}
		}

		product := models.Product{
			ID:           primitive.NewObjectID(),
			Name:         name,
			Price:        price,
			Quantity:     quantity,
			Description:  description,
			Images:       []string{image},
			CategoryID:   categoryIDs[0], // use the first as primary
			CategoryIDs:  categoryIDs,
			CategoryPath: dedupedPaths,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		products = append(products, product)
	}

	var inserts []interface{}
	for _, p := range products {
		inserts = append(inserts, p)
	}

	_, err = database.DB.Collection("products").InsertMany(c, inserts)
	if err != nil {
		log.Println("Error inserting products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bulk insert failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Products inserted successfully", "count": len(products)})
}
