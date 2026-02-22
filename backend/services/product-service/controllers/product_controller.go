package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"product-service/models"
	"product-service/services"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ErrNotFound is returned when a resource is not found
var ErrNotFound = errors.New("not found")

type ProductController struct {
	productService ProductServiceAPI
	redis          *redis.Client
	config         Config
	cache          *CacheManager
	validator      *RequestValidator
}

func NewProductController(ps ProductServiceAPI, redis *redis.Client) *ProductController {
	return &ProductController{
		productService: ps,
		redis:          redis,
		config: Config{
			CacheTTL:       DefaultCacheTTL,
			ContextTimeout: DefaultContextTimeout,
		},
		cache:     NewCacheManager(redis),
		validator: NewRequestValidator(),
	}
}

// GetService returns the product service instance
func (ctrl *ProductController) GetService() ProductServiceAPI {
	return ctrl.productService
}

// GetRedis returns the Redis client
func (ctrl *ProductController) GetRedis() *redis.Client {
	return ctrl.redis
}

// GetCache returns the cache manager
func (ctrl *ProductController) GetCache() *CacheManager {
	return ctrl.cache
}

// GetValidator returns the request validator
func (ctrl *ProductController) GetValidator() *RequestValidator {
	return ctrl.validator
}

// WithCacheTTL sets the cache TTL
func (ctrl *ProductController) WithCacheTTL(ttl time.Duration) *ProductController {
	ctrl.config.CacheTTL = ttl
	ctrl.cache.ttl = ttl
	return ctrl
}

// WithContextTimeout sets the context timeout
func (ctrl *ProductController) WithContextTimeout(timeout time.Duration) *ProductController {
	ctrl.config.ContextTimeout = timeout
	return ctrl
}

// GetProductByID retrieves a single product by ID
func (ctrl *ProductController) GetProductByID(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.config.ContextTimeout)
	defer cancel()

	// Try cache first
	cacheKey := ProductCachePrefix + productID.String()
	if cached, err := ctrl.redis.Get(ctx, cacheKey).Result(); err == nil {
		var product models.Product
		if err := json.Unmarshal([]byte(cached), &product); err == nil {
			zap.L().Debug("Returning product from cache", zap.String("id", id))
			c.JSON(http.StatusOK, product)
			return
		}
	}

	// Cache miss - fetch from service
	product, err := ctrl.productService.GetProduct(ctx, productID)
	if err != nil {
		handleServiceError(c, err, "Product not found")
		return
	}

	// Cache the product asynchronously
	ctrl.cache.SetProductAsync(productID.String(), product)

	c.JSON(http.StatusOK, product)
}

// GetProducts retrieves a paginated list of products with filters
func (ctrl *ProductController) GetProducts(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.config.ContextTimeout)
	defer cancel()

	// Parse and validate parameters
	page, perPage, err := ctrl.validator.ParsePagination(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse filters
	filters, err := ctrl.validator.ParseFilters(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check cache
	response, found := ctrl.cache.GetProductList(ctx, page, perPage, filters)
	if found {
		zap.L().Debug("Returning products from cache")
		c.JSON(http.StatusOK, response)
		return
	}

	// Cache miss - fetch from service
	params := buildServiceParams(page, perPage, filters)
	products, total, err := ctrl.productService.ListProducts(ctx, params)
	if err != nil {
		zap.L().Error("Failed to fetch products from service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}

	// Build response
	response = buildProductListResponse(products, total, page, perPage)

	// Cache the response asynchronously
	ctrl.cache.SetProductListAsync(page, perPage, filters, response)

	c.JSON(http.StatusOK, response)
}

// CreateProduct creates a new product
func (ctrl *ProductController) CreateProduct(c *gin.Context) {
	// Parse and validate request
	req, images, err := ctrl.validator.ParseCreateProductRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.config.ContextTimeout)
	defer cancel()

	// Create product
	product, err := ctrl.productService.CreateProduct(ctx, req, images)
	if err != nil {
		handleCreateError(c, err)
		return
	}

	// Invalidate cache
	if err := ctrl.cache.Invalidate(ctx); err != nil {
		zap.L().Error("CRITICAL: Failed to invalidate cache after product creation",
			zap.Error(err),
			zap.String("product_id", product.ID.String()))
	}

	c.JSON(http.StatusCreated, product)
}

// UpdateProduct updates an existing product
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

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.config.ContextTimeout)
	defer cancel()

	modifiedCount, err := ctrl.productService.UpdateProduct(ctx, productID, updates)
	if err != nil {
		zap.L().Error("Service failed to update product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or no changes made"})
		return
	}

	// Invalidate cache
	ctrl.cache.InvalidateProduct(ctx, productID.String())

	c.JSON(http.StatusOK, gin.H{"message": "Product updated successfully"})
}

// DeleteProduct deletes a product
func (ctrl *ProductController) DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.config.ContextTimeout)
	defer cancel()

	modifiedCount, err := ctrl.productService.DeleteProduct(ctx, productID)
	if err != nil {
		zap.L().Error("Service failed to delete product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	// Invalidate cache
	ctrl.cache.InvalidateProduct(ctx, productID.String())

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}

// GetProductByIDInternal retrieves internal product details
func (ctrl *ProductController) GetProductByIDInternal(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.config.ContextTimeout)
	defer cancel()

	productDTO, err := ctrl.productService.GetProductInternal(ctx, productID)
	if err != nil {
		handleServiceError(c, err, "Product not found")
		return
	}

	c.JSON(http.StatusOK, productDTO)
}

// Helper functions

func handleServiceError(c *gin.Context, err error, notFoundMsg string) {
	if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusNotFound, gin.H{"error": notFoundMsg})
		return
	}
	zap.L().Error("Service error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
}

func handleCreateError(c *gin.Context, err error) {
	zap.L().Error("Service failed to create product", zap.Error(err))
	if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "already exists") {
		c.JSON(http.StatusConflict, gin.H{"error": "Product with this SKU already exists"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
}

func buildServiceParams(page, perPage int, filters *ProductFilters) services.ListProductsParams {
	params := services.ListProductsParams{
		Page:    page,
		PerPage: perPage,
		Sort:    filters.SortParam,
	}

	if filters.IsFeaturedParsed != nil {
		params.IsFeatured = filters.IsFeaturedParsed
	}
	if len(filters.CategoryIDs) > 0 {
		params.CategoryID = filters.CategoryIDs
	}
	if filters.MinPrice != nil {
		params.MinPrice = filters.MinPrice
	}
	if filters.MaxPrice != nil {
		params.MaxPrice = filters.MaxPrice
	}

	return params
}

func buildProductListResponse(products []*models.Product, total int64, page, perPage int) map[string]interface{} {
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	return map[string]interface{}{
		"products": products,
		"meta": map[string]interface{}{
			"page":       page,
			"perPage":    perPage,
			"total":      total,
			"totalPages": totalPages,
		},
	}
}
