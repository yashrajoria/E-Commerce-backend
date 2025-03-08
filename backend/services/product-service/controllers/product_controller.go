package controllers

import (
	"log"
	"net/http"
	"strconv"

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

	// Pagination parameters
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit <= 0 {
		limit = 10
	}
	skip, err := strconv.Atoi(c.DefaultQuery("skip", "0"))
	if err != nil || skip < 0 {
		skip = 0
	}

	// MongoDB query options
	findOptions := options.Find()
	findOptions.SetLimit(int64(limit))
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
		c.JSON(http.StatusInternalServerError, gin.H{"erroar": "Failed to decode products"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"products": products})
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

// CreateProduct inserts a new product into the database.
func CreateProduct(c *gin.Context) {
	var product models.Product

	if err := c.ShouldBindJSON(&product); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	// Validate category exists
	var category models.Category
	err := database.DB.Collection("categories").FindOne(c, bson.M{"_id": product.Category}).Decode(&category)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	product.ID = primitive.NewObjectID()
	_, err = database.DB.Collection("products").InsertOne(c, product)
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
