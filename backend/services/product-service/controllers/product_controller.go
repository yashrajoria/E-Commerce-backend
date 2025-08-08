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

	"github.com/cloudinary/cloudinary-go"
	"github.com/cloudinary/cloudinary-go/api/uploader"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

// GetProducts retrieves paginated products with soft-delete filtering.
func GetProducts(c *gin.Context) {
	collection := database.DB.Collection("products")

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	perPage, err := strconv.Atoi(c.DefaultQuery("perPage", "10"))
	if err != nil || perPage <= 0 {
		perPage = 10
	}
	skip := (page - 1) * perPage

	filter := bson.M{"deleted_at": bson.M{"$exists": false}}

	findOptions := options.Find()
	findOptions.SetLimit(int64(perPage))
	findOptions.SetSkip(int64(skip))

	cursor, err := collection.Find(c, filter, findOptions)
	if err != nil {
		zap.L().Error("Error finding products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)

	var products []models.Product
	if err := cursor.All(c, &products); err != nil {
		zap.L().Error("Error decoding products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	total, err := collection.CountDocuments(c, filter)
	if err != nil {
		zap.L().Error("Error counting products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	zap.L().Info("Products fetched successfully",
		zap.Int("page", page),
		zap.Int("perPage", perPage),
		zap.Int64("total", total),
		zap.Int("totalPages", totalPages),
	)

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

// GetProductByID retrieves a single product by ID with soft-delete check.
func GetProductByID(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		zap.L().Warn("Invalid UUID format", zap.String("id", id))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	filter := bson.M{"_id": productID, "deleted_at": bson.M{"$exists": false}}

	var product models.Product
	err = database.DB.Collection("products").FindOne(c, filter).Decode(&product)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			zap.L().Info("Product not found", zap.String("id", id))
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		} else {
			zap.L().Error("Database error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		}
		return
	}

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

func credentials() (*cloudinary.Cloudinary, context.Context, error) {
	cld, err := cloudinary.New()

	if err != nil {
		return nil, nil, fmt.Errorf("cloudinary init error: %w", err)
	}
	cld.Config.URL.Secure = true
	ctx := context.Background()
	return cld, ctx, nil
}

// CreateProduct creates a new product, uploading images and validating inputs.
func CreateProduct(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		zap.L().Warn("Failed to parse multipart form", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart form"})
		return
	}

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
	is_featured := form.Value["is_featured"]

	requiredFields := map[string][]string{
		"name":        name,
		"category":    category,
		"price":       priceStr,
		"quantity":    quantityStr,
		"description": description,
		"brand":       brand,
		"sku":         sku,
		"is_featured": is_featured,
	}

	for field, value := range requiredFields {
		if len(value) == 0 {
			zap.L().Warn("Missing field", zap.String("field", field))
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Missing required field: %s", field)})
			return
		}
	}

	price, err := strconv.ParseFloat(priceStr[0], 64)
	if err != nil || price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid price"})
		return
	}

	quantity, err := strconv.Atoi(quantityStr[0])
	if err != nil || quantity < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid quantity"})
		return
	}

	isFeaturedBool, err := strconv.ParseBool(is_featured[0])
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid is_featured value"})
		return
	}

	// Parse categories JSON array
	var categoryNames []string
	if err := json.Unmarshal([]byte(category[0]), &categoryNames); err != nil || len(categoryNames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or empty category format"})
		return
	}

	var categoryIDs []uuid.UUID
	var categoryPaths []string
	categorySet := make(map[uuid.UUID]bool)
	pathSet := make(map[string]bool)

	for _, catName := range categoryNames {
		var cat models.Category
		err := database.DB.Collection("categories").FindOne(ctx, bson.M{"name": catName}).Decode(&cat)
		if err != nil {
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

	cld, ctx, err := credentials()
	if err != nil {
		zap.L().Error("Cloudinary init failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Image upload service failed"})
		return
	}
	cld.Config.URL.Secure = true

	var imageURLs []string
	for i, fileHeader := range images {
		file, err := fileHeader.Open()
		if err != nil {
			zap.L().Warn("Failed to open image", zap.Int("imageIndex", i), zap.Error(err))
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
			zap.L().Warn("Image upload error", zap.Int("imageIndex", i), zap.Error(err))
			continue
		}
		if uploadResp == nil || uploadResp.SecureURL == "" {
			zap.L().Warn("Image upload returned empty response", zap.Int("imageIndex", i))
			continue
		}
		imageURLs = append(imageURLs, uploadResp.SecureURL)
	}

	now := time.Now().UTC()
	product := models.Product{
		ID:           uuid.New(),
		Name:         name[0],
		Price:        price,
		Quantity:     quantity,
		Description:  description[0],
		Images:       imageURLs,
		Brand:        brand[0],
		SKU:          sku[0],
		CategoryIDs:  categoryIDs,
		CategoryPath: categoryPaths,
		IsFeatured:   isFeaturedBool,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = database.DB.Collection("products").InsertOne(ctx, product)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "Product with this SKU already exists"})
			return
		}
		zap.L().Error("Failed to insert product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save product"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Product created successfully", "product": product})
}

// UpdateProduct updates an existing product, setting updated_at automatically.
func UpdateProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
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

	updates["updated_at"] = time.Now().UTC()

	result, err := database.DB.Collection("products").UpdateOne(c,
		bson.M{"_id": productID, "deleted_at": bson.M{"$exists": false}},
		bson.M{"$set": updates},
	)
	if err != nil {
		zap.L().Error("Error updating product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}

	if result.ModifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or no changes made"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product updated successfully"})
}

// DeleteProduct performs a soft delete by setting deleted_at timestamp.
func DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	result, err := database.DB.Collection("products").UpdateOne(
		c,
		bson.M{"_id": productID, "deleted_at": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"deleted_at": time.Now().UTC()}},
	)
	if err != nil {
		zap.L().Error("Failed to soft delete product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}
	if result.ModifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or already deleted"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}

// CreateBulkProducts parses CSV and inserts products with input validation and soft delete compliance.
func CreateBulkProducts(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		zap.L().Warn("File upload required", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		zap.L().Error("Failed to open file", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	r := csv.NewReader(src)
	records, err := r.ReadAll()
	if err != nil {
		zap.L().Error("Failed to read CSV", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read CSV"})
		return
	}

	if len(records) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No product records in CSV"})
		return
	}
	records = records[1:] // Skip header

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
		categorySet := make(map[uuid.UUID]bool)

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

		pathSet := make(map[string]bool)
		var dedupedPaths []string
		for _, p := range categoryPaths {
			if !pathSet[p] {
				dedupedPaths = append(dedupedPaths, p)
				pathSet[p] = true
			}
		}

		isFeaturedBool, err := strconv.ParseBool(row[6])
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid is_featured value"})
			return
		}

		now := time.Now().UTC()
		product := models.Product{
			ID:           uuid.New(),
			Name:         name,
			Price:        price,
			Quantity:     quantity,
			Description:  description,
			Images:       []string{image},
			CategoryIDs:  categoryIDs,
			CategoryPath: dedupedPaths,
			IsFeatured:   isFeaturedBool,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		products = append(products, product)
	}

	var inserts []interface{}
	for _, p := range products {
		inserts = append(inserts, p)
	}

	_, err = database.DB.Collection("products").InsertMany(c, inserts)
	if err != nil {
		zap.L().Error("Bulk insert failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bulk insert failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Products inserted successfully", "count": len(products)})
}

// GetProductsByCategory returns paginated products for the specified category, with soft delete filtering.
func GetProductsByCategory(c *gin.Context) {
	categoryID := c.Param("categoryId")
	categoryUUID, err := uuid.Parse(categoryID)
	if err != nil {
		zap.L().Warn("Invalid category ID format", zap.String("id", categoryID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	perPage, err := strconv.Atoi(c.DefaultQuery("perPage", "10"))
	if err != nil || perPage <= 0 {
		perPage = 10
	}
	skip := (page - 1) * perPage

	findOptions := options.Find()
	findOptions.SetLimit(int64(perPage))
	findOptions.SetSkip(int64(skip))

	filter := bson.M{
		"category_ids": categoryUUID,
		"deleted_at":   bson.M{"$exists": false},
	}

	cursor, err := database.DB.Collection("products").Find(c, filter, findOptions)
	if err != nil {
		zap.L().Error("Error finding products by category", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)

	var products []models.Product
	if err := cursor.All(c, &products); err != nil {
		zap.L().Error("Error decoding products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode products"})
		return
	}

	total, err := database.DB.Collection("products").CountDocuments(c, filter)
	if err != nil {
		zap.L().Error("Error counting products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	zap.L().Info("Products fetched by category",
		zap.String("categoryId", categoryID),
		zap.Int("page", page),
		zap.Int("perPage", perPage),
		zap.Int64("total", total),
		zap.Int("totalPages", totalPages),
	)

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
