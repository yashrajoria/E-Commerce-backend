package controllers

import (
	"encoding/csv"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetProducts retrieves paginated products from the database.
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
	Name        string   `json:"title" binding:"required"`
	Price       float64  `json:"price" binding:"required"`
	Category    string   `json:"category" binding:"required"`
	Images      []string `json:"images"`
	Quantity    int      `json:"quantity"`
	Description string   `json:"description"`
}

func CreateProduct(c *gin.Context) {
	var input ProductInput

	// Bind and validate JSON input
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("Invalid JSON body:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	// Convert category ID from string to ObjectID
	catID, err := primitive.ObjectIDFromHex(input.Category)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	// Validate category exists
	ctx := c.Request.Context()
	var category models.Category
	err = database.DB.Collection("categories").FindOne(ctx, bson.M{"_id": catID}).Decode(&category)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	// Create product object for insertion
	product := models.Product{
		ID:          primitive.NewObjectID(),
		Name:        input.Name,
		Price:       input.Price,
		Category:    catID,
		Images:      input.Images,
		Quantity:    input.Quantity,
		Description: input.Description,
	}

	// Insert product into DB
	_, err = database.DB.Collection("products").InsertOne(ctx, product)
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
	// Step 1: Upload and open the file
	file, err := c.FormFile("file")
	if err != nil {
		log.Println("Error getting file:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload required"})
		return
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		log.Println("Error opening file:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	// Step 2: Read CSV data
	r := csv.NewReader(src)
	records, err := r.ReadAll()
	if err != nil {
		log.Println("Error reading CSV:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read CSV"})
		return
	}

	// Step 3: Skip the header row (first row)
	if len(records) > 0 {
		records = records[1:]
	}

	// Step 4: Validate records
	if len(records) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No records found in CSV"})
		return
	}

	var products []models.Product
	var categoryIDs map[string]primitive.ObjectID = make(map[string]primitive.ObjectID)

	// Step 5: Process each record
	for _, row := range records {
		// Basic field validation
		if len(row) < 5 {
			log.Println("Invalid row format:", row)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CSV row format"})
			return
		}

		// Price validation
		priceStr := strings.TrimSpace(row[1])
		if priceStr == "" {
			log.Println("Price is empty for product:", row[0])
			c.JSON(http.StatusBadRequest, gin.H{"error": "Price cannot be empty"})
			return
		}

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			log.Printf("Error parsing price for %s: %v\n", row[0], err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid price format"})
			return
		}

		// Quantity validation
		quantityStr := strings.TrimSpace(row[4])
		if quantityStr == "" {
			log.Println("Quantity is empty for product:", row[0])
			c.JSON(http.StatusBadRequest, gin.H{"error": "Quantity cannot be empty"})
			return
		}

		quantity, err := strconv.Atoi(quantityStr)
		if err != nil {
			log.Printf("Error parsing quantity for %s: %v\n", row[0], err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quantity format"})
			return
		}

		// Handle category - if it doesn't exist, create a new ID
		categoryName := strings.TrimSpace(row[2]) // Assuming category name is in column 3
		var categoryID primitive.ObjectID
		if id, exists := categoryIDs[categoryName]; exists {
			categoryID = id // Use existing category ID
		} else {
			categoryID = primitive.NewObjectID() // Assign new category ID
			categoryIDs[categoryName] = categoryID
		}

		// Create the product object
		product := models.Product{
			ID:          primitive.NewObjectID(),
			Name:        row[0],
			Price:       price,
			Category:    categoryID,
			Images:      []string{row[2]}, // Assuming image URL is in column 3
			Quantity:    quantity,
			Description: row[3],
		}
		products = append(products, product)
	}

	// Step 6: Insert products into the database
	var productInterfaces []interface{}
	for _, product := range products {
		productInterfaces = append(productInterfaces, product)
	}

	_, err = database.DB.Collection("products").InsertMany(c, productInterfaces)
	if err != nil {
		log.Println("Error inserting products:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert products"})
		return
	}

	// Step 7: Return success response
	c.JSON(http.StatusOK, gin.H{"message": "Products inserted successfully"})
}
