package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"product-service/models"
	"product-service/services"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrNotFound is returned when a resource is not found
var ErrNotFound = errors.New("not found")

// Use a single instance of Validate, it caches struct info
var validate = validator.New()

// Validation constants
const (
	MaxPageSize   = 100
	MaxPageNumber = 1000000
	MaxUploadSize = 50 * 1024 * 1024 // 50MB
)

type ProductServiceAPI interface {
	GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error)
	ListProducts(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error)
	CreateProduct(ctx context.Context, req services.ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error)
	UpdateProduct(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (int64, error)
	DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error)
	GetProductInternal(ctx context.Context, id uuid.UUID) (*services.ProductInternalDTO, error)
	ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error)
	ProcessBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportResult, error)
	GeneratePresignedUpload(ctx context.Context, sku, filename, contentType string, expiresSeconds int64) (string, string, string, error)
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
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
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
	// 1. Parse Parameters with validation
	pageStr := c.DefaultQuery("page", "1")
	perPageStr := c.DefaultQuery("perPage", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page number"})
		return
	}
	if page > MaxPageNumber {
		page = MaxPageNumber
	}

	perPage, err := strconv.Atoi(perPageStr)
	if err != nil || perPage < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page size"})
		return
	}
	if perPage > MaxPageSize {
		perPage = MaxPageSize
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
	// The key MUST include every variable that changes the output.
	// We include a `products:version` value so we can invalidate all
	// product-list caches atomically by bumping the version on writes.
	var cacheVersion int64 = 1
	if ver, err := ctrl.redis.Get(c.Request.Context(), "products:version").Int64(); err == nil && ver > 0 {
		cacheVersion = ver
	} else if err != nil && err != redis.Nil {
		zap.L().Warn("failed to read products cache version", zap.Error(err))
	}

	cacheKey := fmt.Sprintf(
		"products:v:%d:p:%d:l:%d:f:%s:c:%s:s:%s:min:%s:max:%s",
		cacheVersion,
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
	} else if err != redis.Nil {
		// Log Redis errors other than 'key not found'
		zap.L().Error("Redis error while fetching cache", zap.Error(err), zap.String("cacheKey", cacheKey))
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

	// Invalidate cache after creating a product
	// WARNING: FlushDB clears the ENTIRE Redis instance. Use specific key invalidation in production.
	// TODO: Implement pattern-based deletion (e.g. SCAN for "products:*") or versioning.
	// if err := ctrl.redis.FlushDB(c.Request.Context()).Err(); err != nil {
	// 	zap.L().Error("failed to invalidate cache after product creation", zap.Error(err))
	// }

	// Bump the products cache version so existing product-list cache entries
	// become stale immediately (they include the version in their key).
	if err := ctrl.redis.Incr(c.Request.Context(), "products:version").Err(); err != nil {
		zap.L().Warn("failed to bump products cache version after create", zap.Error(err))
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

	var updates map[string]interface{}
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

	// Invalidate cache after updating a product
	// if err := ctrl.redis.FlushDB(c.Request.Context()).Err(); err != nil {
	// 	zap.L().Error("failed to invalidate cache after product update", zap.Error(err))
	// }

	// Bump the products cache version so list caches become stale.
	if err := ctrl.redis.Incr(c.Request.Context(), "products:version").Err(); err != nil {
		zap.L().Warn("failed to bump products cache version after update", zap.Error(err))
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

	// Invalidate cache after deleting a product
	// if err := ctrl.redis.FlushDB(c.Request.Context()).Err(); err != nil {
	// 	zap.L().Error("failed to invalidate cache after product deletion", zap.Error(err))
	// }

	// Bump products cache version to invalidate list caches atomically.
	if err := ctrl.redis.Incr(c.Request.Context(), "products:version").Err(); err != nil {
		zap.L().Warn("failed to bump products cache version after delete", zap.Error(err))
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

	if file.Size > MaxUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (max 50MB)"})
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

	if file.Size > MaxUploadSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (max 50MB)"})
		return
	}

	// Support async processing via query param ?async=true
	async := strings.ToLower(strings.TrimSpace(c.Query("async"))) == "true"

	fileHandle, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to open file",
		})
		return
	}
	defer fileHandle.Close()

	if async {
		// Persist uploaded file to disk for worker processing
		data, err := io.ReadAll(fileHandle)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file for async processing"})
			return
		}

		storageDir := os.Getenv("BULK_STORAGE_DIR")
		if storageDir == "" {
			storageDir = "./data/bulk_imports"
		}
		if err := os.MkdirAll(storageDir, 0o755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create storage dir"})
			return
		}

		jobID := uuid.New().String()
		filename := fmt.Sprintf("%s.csv", jobID)
		path := fmt.Sprintf("%s/%s", strings.TrimRight(storageDir, "/"), filename)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to persist file for async processing"})
			return
		}

		jobIDStr := jobID
		jobKey := fmt.Sprintf("bulk_import:job:%s", jobIDStr)
		jobInfo := map[string]interface{}{"status": "pending", "created_at": time.Now().UTC().Format(time.RFC3339), "file_path": path}
		b, _ := json.Marshal(jobInfo)
		if err := ctrl.redis.Set(c.Request.Context(), jobKey, b, 24*time.Hour).Err(); err != nil {
			os.Remove(path)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
			return
		}

		// Push job ID to queue for worker(s)
		queueKey := "bulk_import:queue"
		if err := ctrl.redis.RPush(c.Request.Context(), queueKey, jobIDStr).Err(); err != nil {
			os.Remove(path)
			ctrl.redis.Del(c.Request.Context(), jobKey)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue job"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{"job_id": jobIDStr})
		return
	}

	// synchronous path
	result, err := ctrl.productService.ProcessBulkImport(c.Request.Context(), fileHandle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetBulkImportJobStatus returns the job status/result stored in Redis
func (ctrl *ProductController) GetBulkImportJobStatus(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job id required"})
		return
	}
	jobKey := fmt.Sprintf("bulk_import:job:%s", id)
	val, err := ctrl.redis.Get(c.Request.Context(), jobKey).Result()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(val), &out); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse job result"})
		return
	}
	c.JSON(http.StatusOK, out)
}

// GetPresignUpload returns a presigned URL for direct S3 upload and the public URL
func (ctrl *ProductController) GetPresignUpload(c *gin.Context) {
	sku := c.Query("sku")
	if strings.TrimSpace(sku) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sku query parameter is required"})
		return
	}

	filename := c.DefaultQuery("filename", "upload")
	contentType := c.DefaultQuery("content_type", "application/octet-stream")
	expiresStr := c.DefaultQuery("expires", "900")
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || expires <= 0 {
		expires = 900
	}

	uploadURL, key, publicURL, err := ctrl.productService.GeneratePresignedUpload(c.Request.Context(), sku, filename, contentType, expires)
	if err != nil {
		zap.L().Error("failed to generate presigned upload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate presigned upload"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": uploadURL,
		"method":     "PUT",
		"key":        key,
		"public_url": publicURL,
	})
}

// PostPresignUpload returns a presigned URL for PUT upload for a specific product ID.
func (ctrl *ProductController) PostPresignUpload(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	// ensure product exists
	_, err = ctrl.productService.GetProduct(c.Request.Context(), productID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		zap.L().Error("Service failed to get product", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	// prepare presign
	filename := c.DefaultQuery("filename", "upload")
	expiresStr := c.DefaultQuery("expires", "900")
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || expires <= 0 {
		expires = 900
	}

	// load aws config and generate presign
	cfg, err := aws_pkg.LoadAWSConfig(c.Request.Context())
	if err != nil {
		zap.L().Error("failed to load aws config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AWS config error"})
		return
	}

	bucket := os.Getenv("S3_BUCKET_IMAGES")
	if bucket == "" {
		bucket = "ecommerce-product-images"
	}

	key := fmt.Sprintf("product/%s/%s", productID.String(), filename)
	url, _, err := aws_pkg.GeneratePresignedPutURL(c.Request.Context(), cfg, bucket, key, expires)
	if err != nil {
		zap.L().Error("failed to generate presigned url", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate presigned upload"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"upload_url": url, "method": "PUT", "key": key, "expires_in": expires})
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
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
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
