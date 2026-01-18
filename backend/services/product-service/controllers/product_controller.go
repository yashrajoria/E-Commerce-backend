package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"product-service/models"
	"product-service/services"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Use a single instance of Validate, it caches struct info
var validate = validator.New()

type ProductServiceAPI interface {
	GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error)
	ListProducts(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error)
	CreateProduct(ctx context.Context, req services.ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error)
	UpdateProduct(ctx context.Context, id uuid.UUID, updates bson.M) (int64, error)
	DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error)
	GetProductInternal(ctx context.Context, id uuid.UUID) (*services.ProductInternalDTO, error)
	ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error)
	CreateBulkProducts(ctx context.Context, file multipart.File, autoCreateCategories bool) (*models.BulkImportResult, error)
}

// CreateProductRequest defines the expected structure for creating a product via multipart-form.
type CreateProductRequest struct {
	Name        string  `form:"name" validate:"required"`
	Description string  `form:"description" validate:"required"`
	Brand       string  `form:"brand" validate:"required"`
	SKU         string  `form:"sku" validate:"required"`
	Price       float64 `form:"price" validate:"required,gt=0"`
	Quantity    int     `form:"quantity" validate:"required,gte=0"`
	IsFeatured  bool    `form:"is_featured"`
	Categories  string  `form:"category" validate:"required"` // JSON string array
}

type ProductController struct {
	productService ProductServiceAPI
	redis          *redis.Client
}

func NewProductController(ps ProductServiceAPI, redis *redis.Client) *ProductController {
	return &ProductController{
		productService: ps,
		redis:          redis,
	}
}

func (ctrl *ProductController) GetProductByID(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	product, err := ctrl.productService.GetProduct(c.Request.Context(), productID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		zap.L().Error("Service failed to get product", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}
	c.JSON(http.StatusOK, product)
}

func (ctrl *ProductController) GetProducts(c *gin.Context) {
	// 1. Parse Parameters (Same as before)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("perPage", "10"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 10
	}

	// Parse filters for the Cache Key
	isFeatured := c.Query("is_featured")
	normalizedIsFeatured := strings.ToLower(strings.TrimSpace(isFeatured))
	categoryIDsParam := c.Query("categoryId")
	normalizedCategoryKey := ""
	var categoryIDs []uuid.UUID
	if categoryIDsParam != "" {
		rawIDs := strings.Split(categoryIDsParam, ",")
		var categoryIDStrings []string
		for _, rawID := range rawIDs {
			trimmed := strings.TrimSpace(rawID)
			if trimmed == "" {
				continue
			}
			categoryUUID, err := uuid.Parse(trimmed)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
				return
			}
			categoryIDs = append(categoryIDs, categoryUUID)
			categoryIDStrings = append(categoryIDStrings, categoryUUID.String())
		}
		if len(categoryIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
			return
		}
		sort.Strings(categoryIDStrings)
		normalizedCategoryKey = strings.Join(categoryIDStrings, ",")
	}

	sortParam := strings.TrimSpace(c.Query("sort"))
	normalizedSortParam := strings.ToLower(sortParam)
	if sortParam != "" && !isSupportedSort(sortParam) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sort value"})
		return
	}

	var minPrice *float64
	minPriceStr := strings.TrimSpace(c.Query("minPrice"))
	if minPriceStr != "" {
		parsed, err := strconv.ParseFloat(minPriceStr, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid minPrice value"})
			return
		}
		minPrice = &parsed
	}

	var maxPrice *float64
	maxPriceStr := strings.TrimSpace(c.Query("maxPrice"))
	if maxPriceStr != "" {
		parsed, err := strconv.ParseFloat(maxPriceStr, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid maxPrice value"})
			return
		}
		maxPrice = &parsed
	}

	if minPrice != nil && maxPrice != nil && *minPrice > *maxPrice {
		c.JSON(http.StatusBadRequest, gin.H{"error": "minPrice must be less than or equal to maxPrice"})
		return
	}

	// 2. GENERATE A UNIQUE CACHE KEY
	// The key MUST include every variable that changes the output
	cacheKey := fmt.Sprintf(
		"products:p:%d:l:%d:f:%s:c:%s:s:%s:min:%s:max:%s",
		page,
		perPage,
		normalizedIsFeatured,
		normalizedCategoryKey,
		normalizedSortParam,
		formatFloatForCache(minPrice),
		formatFloatForCache(maxPrice),
	)

	// 3. TRY TO GET FROM REDIS
	val, err := ctrl.redis.Get(c.Request.Context(), cacheKey).Result()

	if err == nil {
		// --- CACHE HIT ---
		var cachedResponse map[string]interface{}

		// Unmarshal the JSON string back into a Go map/struct
		if err := json.Unmarshal([]byte(val), &cachedResponse); err == nil {
			zap.L().Info("Returning data from Redis Cache")
			c.JSON(http.StatusOK, cachedResponse)
			return // <--- RETURN IMMEDIATELY, SKIP DB
		}
	}

	// --- CACHE MISS (Proceed to DB) ---

	// Prepare params for Service (Same as before)
	params := services.ListProductsParams{
		Page:    page,
		PerPage: perPage,
		Sort:    sortParam,
	}

	if isFeaturedStr := c.Query("is_featured"); isFeaturedStr != "" {
		isFeatured, err := strconv.ParseBool(isFeaturedStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid boolean value for 'is_featured'"})
			return
		}
		params.IsFeatured = &isFeatured
	}

	if len(categoryIDs) > 0 {
		params.CategoryID = categoryIDs
	}
	if minPrice != nil {
		params.MinPrice = minPrice
	}
	if maxPrice != nil {
		params.MaxPrice = maxPrice
	}

	products, total, err := ctrl.productService.ListProducts(c.Request.Context(), params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	// Construct Response
	response := gin.H{
		"products": products,
		"meta": gin.H{
			"page":       page,
			"perPage":    perPage,
			"total":      total,
			"totalPages": totalPages,
		},
	}

	// 4. SAVE TO REDIS (Serialize to JSON)
	// We store the whole response so we can return it instantly next time
	jsonBytes, err := json.Marshal(response)
	if err == nil {
		// Set TTL to 10 minutes (or whatever fits your needs)
		if err := ctrl.redis.Set(c.Request.Context(), cacheKey, jsonBytes, 10*time.Minute).Err(); err != nil {
			zap.L().Error("failed to cache products response in Redis", zap.Error(err), zap.String("cacheKey", cacheKey))
		}
	}

	c.JSON(http.StatusOK, response)
}

func (ctrl *ProductController) CreateProduct(c *gin.Context) {
	var req CreateProductRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data", "details": err.Error()})
		return
	}

	if err := validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	var categoryNames []string
	if err := json.Unmarshal([]byte(req.Categories), &categoryNames); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category format, must be a JSON string array"})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Expected multipart form data"})
		return
	}
	images := form.File["images"]
	if len(images) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one image is required"})
		return
	}

	serviceReq := services.ProductCreateRequest{
		Name:        req.Name,
		Description: req.Description,
		Brand:       req.Brand,
		SKU:         req.SKU,
		Price:       req.Price,
		Quantity:    req.Quantity,
		IsFeatured:  req.IsFeatured,
		Categories:  categoryNames,
	}

	product, err := ctrl.productService.CreateProduct(c.Request.Context(), serviceReq, images)
	if err != nil {
		zap.L().Error("Service failed to create product", zap.Error(err))
		// You can add more specific error checks here (e.g., for duplicate SKU)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
		return
	}

	c.JSON(http.StatusCreated, product)
}

func (ctrl *ProductController) UpdateProduct(c *gin.Context) {
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

	modifiedCount, err := ctrl.productService.UpdateProduct(c.Request.Context(), productID, updates)
	if err != nil {
		zap.L().Error("Service failed to update product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or no changes made"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product updated successfully"})
}

func (ctrl *ProductController) DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	modifiedCount, err := ctrl.productService.DeleteProduct(c.Request.Context(), productID)
	if err != nil {
		zap.L().Error("Service failed to delete product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}

// ValidateBulkImport validates CSV before import
func (ctrl *ProductController) ValidateBulkImport(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "File is required",
		})
		return
	}

	fileHandle, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to open file",
		})
		return
	}
	defer fileHandle.Close()

	validation, err := ctrl.productService.ValidateBulkImport(c.Request.Context(), fileHandle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, validation)
}

// CreateBulkProducts imports products from CSV
func (ctrl *ProductController) CreateBulkProducts(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "File is required",
		})
		return
	}

	autoCreate := c.DefaultQuery("auto_create_categories", "false") == "true"

	fileHandle, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to open file",
		})
		return
	}
	defer fileHandle.Close()

	result, err := ctrl.productService.CreateBulkProducts(c.Request.Context(), fileHandle, autoCreate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (ctrl *ProductController) GetProductByIDInternal(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	productDTO, err := ctrl.productService.GetProductInternal(c.Request.Context(), productID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		zap.L().Error("Service failed to get internal product", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, productDTO)
}

func isSupportedSort(sortParam string) bool {
	switch sortParam {
	case "price_asc", "price_desc", "created_at_asc", "created_at_desc", "name_asc", "name_desc":
		return true
	default:
		return false
	}
}

func formatFloatForCache(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}
