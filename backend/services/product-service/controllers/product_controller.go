package controllers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/api/uploader"
	"github.com/google/uuid"

	"github.com/cloudinary/cloudinary-go"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/yashrajoria/product-service/database"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func init() {
	_ = godotenv.Load()
}

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
		log.Println(c, "Error finding products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)
	if err := cursor.All(c, &products); err != nil {
		log.Println(c, "Error decoding products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	total, err := collection.CountDocuments(c, bson.M{})
	if err != nil {
		log.Println(c, "Error counting products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	log.Println(c, "Products fetched successfully",
		zap.Int("page", page),
		zap.Int("perPage", perPage),
		zap.Int64("total", total),
		zap.Int("totalPages", totalPages))

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
	productID, err := uuid.Parse(id)
	log.Println("Fetching product by ID", productID)

	if err != nil {
		log.Println("Invalid UUID format:", id)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	var product models.Product
	err = database.DB.Collection("products").FindOne(c, bson.M{"_id": productID}).Decode(&product)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Println(c, "Product not found", zap.String("id", id))
			c.JSON(http.StatusNotFound, gin.H{"message": "Product not found"})
		} else {
			log.Println(c, "Database error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		}
		return
	}

	log.Println(c, "Product fetched successfully", zap.String("id", id))
	c.JSON(http.StatusOK, product)
}

func GetProductByIDInternal(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		log.Println(c, "Product ID is required")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Product ID is required"})
		return
	}
	productID, err := uuid.Parse(id)
	if err != nil {
		log.Println("Invalid product ID format", zap.String("id", id))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	var product models.Product
	err = database.DB.Collection("products").FindOne(c, bson.M{"_id": productID}).Decode(&product)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Println(c, "Product not found", zap.String("id", id))
			c.JSON(http.StatusNotFound, gin.H{"message": "Product not found"})
		} else {
			log.Println(c, "Database error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		}
		return
	}

	log.Println(c, "Product fetched successfully", zap.String("id", id))
	c.JSON(http.StatusOK, gin.H{
		"id":    product.ID,
		"name":  product.Name,
		"price": product.Price,
		"stock": product.Quantity,
	})
}

type ProductInput struct {
	Name        string   `json:"name" binding:"required"`
	Price       float64  `json:"price" binding:"required"`
	Categories  []string `json:"category" binding:"required"`
	Images      []string `json:"images"`
	Quantity    int      `json:"quantity"`
	Description string   `json:"description"`
	Brand       string   `json:"brand"`
	SKU         string   `json:"sku"`
}

func credentials() (*cloudinary.Cloudinary, context.Context, error) {
	cld, err := cloudinary.New()

	if err != nil {
		return nil, nil, fmt.Errorf("cloudinary init error: %w", err)
	}
	cld.Config.URL.Secure = true
	ctx := context.Background()
	return cld, ctx, nil
}

func CreateProduct(c *gin.Context) {
	log.Println("[CreateProduct] --- Starting product creation ---")

	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		log.Println("[CreateProduct] Failed to parse multipart form:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart form"})
		return
	}
	log.Println("[CreateProduct] Multipart form parsed successfully")

	ctx := c.Request.Context()
	form := c.Request.MultipartForm

	name := form.Value["name"]
	category := form.Value["category"]
	priceStr := form.Value["price"]
	quantityStr := form.Value["quantity"]
	description := form.Value["description"]
	images := form.File["images"]
	brand := form.Value["brand"]
	sku := form.Value["sku"]

	log.Printf("[CreateProduct] Form fields received: name=%v, category=%v, price=%v, quantity=%v", name, category, priceStr, quantityStr)

	requiredFields := map[string][]string{
		"name":        name,
		"category":    category,
		"price":       priceStr,
		"quantity":    quantityStr,
		"description": description,
		"brand":       brand,
		"sku":         sku,
	}

	for field, value := range requiredFields {
		if len(value) == 0 {
			log.Printf("[CreateProduct] Missing field: %s\n", field)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Missing required field: %s", field)})
			return
		}
	}

	price, err := strconv.ParseFloat(priceStr[0], 64)
	if err != nil || price <= 0 {
		log.Println("[CreateProduct] Invalid price:", priceStr[0])
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid price"})
		return
	}

	quantity, err := strconv.Atoi(quantityStr[0])
	if err != nil || quantity < 0 {
		log.Println("[CreateProduct] Invalid quantity:", quantityStr[0])
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quantity"})
		return
	}

	// Parse categories
	var categoryNames []string
	if err := json.Unmarshal([]byte(category[0]), &categoryNames); err != nil || len(categoryNames) == 0 {
		log.Println("[CreateProduct] Failed to parse category JSON:", category[0], "Error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or empty category format"})
		return
	}
	log.Printf("[CreateProduct] Parsed category names: %v", categoryNames)

	var categoryIDs []uuid.UUID
	var categoryPaths []string
	categorySet := make(map[uuid.UUID]bool)
	pathSet := make(map[string]bool)

	for _, catName := range categoryNames {
		log.Printf("[CreateProduct] Looking up category: %s", catName)
		var cat models.Category
		err := database.DB.Collection("categories").FindOne(ctx, bson.M{"name": catName}).Decode(&cat)
		if err != nil {
			log.Printf("[CreateProduct] Category not found: %s. Error: %v", catName, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Category '%s' not found", catName)})
			return
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
		for _, path := range cat.Path {
			if !pathSet[path] {
				categoryPaths = append(categoryPaths, path)
				pathSet[path] = true
			}
		}
	}
	log.Printf("[CreateProduct] Final category IDs: %v", categoryIDs)
	log.Printf("[CreateProduct] Final category paths: %v", categoryPaths)

	// Cloudinary setup
	cld, ctx, err := credentials()
	if err != nil {
		log.Println("[CreateProduct] Cloudinary init failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Image upload service failed"})
		return
	}
	log.Println("[CreateProduct] Cloudinary initialized successfully")

	// Upload images
	var imageURLs []string
	for i, fileHeader := range images {
		log.Printf("[CreateProduct] Uploading image %d: %s", i, fileHeader.Filename)

		file, err := fileHeader.Open()
		if err != nil {
			log.Printf("[CreateProduct] Failed to open image %d: %v", i, err)
			continue
		}

		uploadParams := uploader.UploadParams{
			PublicID:  fmt.Sprintf("product_img_%d_%d", time.Now().Unix(), i),
			Folder:    "ecommerce/products",
			Overwrite: true,
		}

		uploadResp, err := cld.Upload.Upload(ctx, file, uploadParams)
		file.Close()

		if err != nil {
			log.Printf("[CreateProduct] Image %d upload error: %v", i, err)
			continue
		}
		if uploadResp == nil || uploadResp.SecureURL == "" {
			log.Printf("[CreateProduct] Image %d upload returned empty response", i)
			continue
		}

		log.Printf("[CreateProduct] Image %d uploaded: %s", i, uploadResp.SecureURL)
		imageURLs = append(imageURLs, uploadResp.SecureURL)
	}

	log.Printf("[CreateProduct] All image uploads completed. Total: %d", len(imageURLs))

	product := models.Product{
		ID:           uuid.New(),
		Name:         name[0],
		Price:        price,
		Quantity:     quantity,
		Description:  description[0],
		Images:       imageURLs,
		Brand:        brand[0],
		SKU:          sku[0],
		CategoryID:   categoryIDs[0],
		CategoryIDs:  categoryIDs,
		CategoryPath: categoryPaths,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	log.Printf("[CreateProduct] Attempting to insert product: %+v", product)

	_, err = database.DB.Collection("products").InsertOne(ctx, product)
	if err != nil {
		log.Println("[CreateProduct] Failed to insert product:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save product"})
		return
	}

	log.Println("[CreateProduct] Product inserted successfully:", product.ID)
	c.JSON(http.StatusCreated, gin.H{"message": "Product created successfully", "product": product})
}

// DeleteProduct removes a product by ID.
func DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		log.Println("Invalid UUID format:", id)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	result, err := database.DB.Collection("products").DeleteOne(c, bson.M{"_id": productID})
	if err != nil {
		log.Println(c, "Error deleting product", zap.Error(err))
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
	productID, err := uuid.Parse(id)
	if err != nil {
		log.Println("Invalid UUID format:", id)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
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

	result, err := database.DB.Collection("products").UpdateOne(c, bson.M{"_id": productID}, bson.M{"$set": updates})
	if err != nil {
		log.Println(c, "Error updating product", zap.Error(err))
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
		log.Println(c, "Error getting file", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		log.Println(c, "Error opening file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	r := csv.NewReader(src)
	records, err := r.ReadAll()
	if err != nil {
		log.Println(c, "Error reading CSV", zap.Error(err))
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

		var categoryIDs []uuid.UUID
		var categoryPaths []string
		categorySet := make(map[uuid.UUID]bool) // to dedupe ancestors

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
			ID:           uuid.New(),
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
		log.Println(c, "Error inserting products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bulk insert failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Products inserted successfully", "count": len(products)})
}

// GetProductsByCategory retrieves all products belonging to a specific category
func GetProductsByCategory(c *gin.Context) {
	categoryID := c.Param("categoryId")
	categoryUUID, err := uuid.Parse(categoryID)
	if err != nil {
		log.Println(c, "Invalid category ID format", zap.String("id", categoryID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	// Parse query parameters for pagination
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

	// Find products that belong to this category
	filter := bson.M{
		"category_ids": categoryUUID,
	}

	var products []models.Product
	cursor, err := database.DB.Collection("products").Find(c, filter, findOptions)
	if err != nil {
		log.Println(c, "Error finding products by category", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)

	if err := cursor.All(c, &products); err != nil {
		log.Println(c, "Error decoding products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	// Get total count for pagination
	total, err := database.DB.Collection("products").CountDocuments(c, filter)
	if err != nil {
		log.Println(c, "Error counting products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	log.Println(c, "Products fetched by category successfully",
		zap.String("categoryId", categoryID),
		zap.Int("page", page),
		zap.Int("perPage", perPage),
		zap.Int64("total", total),
		zap.Int("totalPages", totalPages))

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
