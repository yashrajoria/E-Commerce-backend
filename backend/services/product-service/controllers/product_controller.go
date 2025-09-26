package controllers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
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

	// Pagination defaults
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

	findOptions := options.Find().
		SetLimit(int64(perPage)).
		SetSkip(int64(skip))

	cursor, err := collection.Find(c, filter, findOptions)
	if err != nil {
		zap.L().Error("Mongo Find failed",
			zap.Error(err),
			zap.Int("page", page),
			zap.Int("perPage", perPage),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}
	defer cursor.Close(c)

	var products []models.Product
	for cursor.Next(c) {
		var p models.Product
		if err := cursor.Decode(&p); err != nil {
			// Skip bad documents but log them
			zap.L().Warn("Skipping malformed product document",
				zap.Error(err),
			)
			continue
		}
		products = append(products, p)
	}
	if err := cursor.Err(); err != nil {
		zap.L().Error("Cursor iteration error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read products"})
		return
	}

	total, err := collection.CountDocuments(c, filter)
	if err != nil {
		zap.L().Error("Counting products failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	zap.L().Info("Products fetched",
		zap.Int("page", page),
		zap.Int("perPage", perPage),
		zap.Int("returned", len(products)),
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
// CreateBulkProducts parses CSV and inserts products with input validation and better error handling.
func CreateBulkProducts(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	r := csv.NewReader(src)

	// --- 1. Parse headers dynamically ---
	headers, err := r.Read()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CSV must include a header row"})
		return
	}

	index := make(map[string]int)
	for i, h := range headers {
		key := strings.ToLower(strings.TrimSpace(h))
		index[key] = i
	}

	requiredCols := []string{"name", "price", "categories", "description", "quantity", "imageurl", "is_featured"}
	for _, col := range requiredCols {
		if _, ok := index[col]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Missing required column: %s", col)})
			return
		}
	}

	// --- 2. Prepare accumulators ---
	var products []models.Product
	var errorsList []map[string]interface{}
	categoryNamesSet := make(map[string]bool)

	// --- 3. Read rows one by one ---
	rowNum := 1
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Failed to parse row",
			})
			rowNum++
			continue
		}

		rowNum++

		// Collect categories for prefetch
		catRaw := strings.Split(row[index["categories"]], ",")
		for _, cName := range catRaw {
			cName = strings.TrimSpace(cName)
			if cName != "" {
				categoryNamesSet[cName] = true
			}
		}
	}

	// --- 4. Prefetch categories from DB ---
	var allCategories []models.Category
	var categoryCache = make(map[string]models.Category)
	if len(categoryNamesSet) > 0 {
		var catNames []string
		for name := range categoryNamesSet {
			catNames = append(catNames, name)
		}
		cursor, err := database.DB.Collection("categories").Find(c, bson.M{"name": bson.M{"$in": catNames}})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
			return
		}
		defer cursor.Close(c)

		if err := cursor.All(c, &allCategories); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode categories"})
			return
		}
		for _, cat := range allCategories {
			categoryCache[cat.Name] = cat
		}
	}

	// --- 5. Re-read rows and build products ---
	// Reset file pointer & reader
	_, _ = src.Seek(0, io.SeekStart)
	r = csv.NewReader(src)
	_, _ = r.Read() // skip headers again

	rowNum = 2 // start after header
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Failed to parse row",
			})
			rowNum++
			continue
		}

		// --- Parse row safely ---
		name := strings.TrimSpace(row[index["name"]])
		price, err1 := strconv.ParseFloat(strings.TrimSpace(row[index["price"]]), 64)
		quantity, err2 := strconv.Atoi(strings.TrimSpace(row[index["quantity"]]))
		image := strings.TrimSpace(row[index["imageurl"]])
		description := strings.TrimSpace(row[index["description"]])
		isFeatured, err3 := strconv.ParseBool(strings.TrimSpace(row[index["is_featured"]]))
		brand := strings.TrimSpace(row[index["brand"]])
		sku := strings.TrimSpace(row[index["sku"]])

		if name == "" || err1 != nil || err2 != nil || err3 != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Invalid data (missing name or bad number formats)",
			})
			rowNum++
			continue
		}

		// --- Map categories ---
		rawCategories := strings.Split(row[index["categories"]], ",")
		var categoryIDs []uuid.UUID
		var categoryPaths []string
		categorySet := make(map[uuid.UUID]bool)

		catValid := true
		for _, rawCat := range rawCategories {
			cName := strings.TrimSpace(rawCat)
			if cName == "" {
				continue
			}

			cat, ok := categoryCache[cName]
			if !ok {
				errorsList = append(errorsList, map[string]interface{}{
					"row":   rowNum,
					"error": fmt.Sprintf("Category '%s' not found", cName),
				})
				catValid = false
				continue
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

		if !catValid {
			rowNum++
			continue
		}

		// Dedup paths
		pathSet := make(map[string]bool)
		var dedupedPaths []string
		for _, p := range categoryPaths {
			if !pathSet[p] {
				dedupedPaths = append(dedupedPaths, p)
				pathSet[p] = true
			}
		}

		now := time.Now().UTC()
		product := models.Product{
			ID:           uuid.New(),
			Name:         name,
			Price:        price,
			Quantity:     quantity,
			Description:  description,
			Images:       []string{image},
			Brand:        brand,
			SKU:          sku,
			CategoryIDs:  categoryIDs,
			CategoryPath: dedupedPaths,
			IsFeatured:   isFeatured,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		products = append(products, product)

		rowNum++
	}

	// --- 6. Insert valid products ---
	if len(products) > 0 {
		var inserts []interface{}
		for _, p := range products {
			inserts = append(inserts, p)
		}
		_, err = database.DB.Collection("products").InsertMany(c, inserts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Bulk insert failed"})
			return
		}
	}

	// --- 7. Final response ---
	c.JSON(http.StatusOK, gin.H{
		"inserted_count": len(products),
		"errors_count":   len(errorsList),
		"errors":         errorsList,
	})
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
