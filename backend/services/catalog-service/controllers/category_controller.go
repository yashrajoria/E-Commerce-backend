package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"catalog-service/models"
	"catalog-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CategoryServiceAPI defines the interface for category service operations
type CategoryServiceAPI interface {
	CreateCategory(ctx context.Context, req services.CategoryCreateRequest) (*models.Category, error)
	GetCategoryTree(ctx context.Context) ([]*models.Category, error)
	ListCategories(ctx context.Context, params services.ListCategoriesParams) (*services.CategoryPage, error)
	UpdateCategory(ctx context.Context, id uuid.UUID, req services.CategoryCreateRequest) (int64, error)
	DeleteCategory(ctx context.Context, id uuid.UUID) error
	GetCategory(ctx context.Context, id uuid.UUID) (*models.Category, error)
	CreateBulkCategories(ctx context.Context, req services.BulkCategoryCreateRequest) ([]*models.Category, error)
}

// CategoryController represents category controller.
type CategoryController struct {
	service   CategoryServiceAPI
	cache     *CacheManager
	validator *RequestValidator
	timeout   time.Duration
}

// NewCategoryController creates and initializes a new CategoryController instance.
func NewCategoryController(s CategoryServiceAPI, redisClient *redis.Client) *CategoryController {
	return &CategoryController{
		service:   s,
		cache:     NewCacheManager(redisClient),
		validator: NewRequestValidator(),
		timeout:   DefaultContextTimeout,
	}
}

func (ctrl *CategoryController) invalidateProductCache(ctx context.Context) {
	if err := ctrl.cache.Invalidate(ctx); err != nil {
		zap.L().Error("Failed to invalidate product cache after category mutation", zap.Error(err))
	}
}

// WithTimeout sets the context timeout for operations
func (ctrl *CategoryController) WithTimeout(timeout time.Duration) *CategoryController {
	ctrl.timeout = timeout
	return ctrl
}

// CreateCategory handles POST /categories (admin only), creating a new category node in the taxonomy.
func (ctrl *CategoryController) CreateCategory(c *gin.Context) {
	var req services.CategoryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := ctrl.validator.validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout)
	defer cancel()

	category, err := ctrl.service.CreateCategory(ctx, req)
	if err != nil {
		handleCategoryCreateError(c, err)
		return
	}

	ctrl.invalidateProductCache(ctx)
	c.JSON(http.StatusCreated, category)
}

// GetCategories handles GET /categories, returning category trees and product count hierarchies.
func (ctrl *CategoryController) GetCategories(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout)
	defer cancel()

	page, perPage, err := ctrl.validator.ParsePagination(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	active := (*bool)(nil)
	if raw := strings.TrimSpace(c.Query("is_active")); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid boolean value for 'is_active'"})
			return
		}
		active = &value
	}
	sortBy := strings.TrimSpace(c.Query("sort"))
	switch sortBy {
	case "", "name_asc", "name_desc", "created_at_asc", "created_at_desc":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sort value"})
		return
	}

	result, err := ctrl.service.ListCategories(ctx, services.ListCategoriesParams{
		Page:    page,
		PerPage: perPage,
		Search:  strings.TrimSpace(c.Query("search")),
		Active:  active,
		Sort:    sortBy,
	})
	if err != nil {
		zap.L().Error("Service failed to get category tree", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetCategory retrieves a single category by ID
func (ctrl *CategoryController) GetCategory(c *gin.Context) {
	id := c.Param("id")
	categoryID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout)
	defer cancel()

	category, err := ctrl.service.GetCategory(ctx, categoryID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		zap.L().Error("Service failed to get category", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, category)
}

// UpdateCategory handles PUT /categories/:id (admin only), modifying category title, slug, or parent.
func (ctrl *CategoryController) UpdateCategory(c *gin.Context) {
	id := c.Param("id")
	categoryID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	var req services.CategoryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := ctrl.validator.validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout)
	defer cancel()

	modifiedCount, err := ctrl.service.UpdateCategory(ctx, categoryID, req)
	if err != nil {
		handleCategoryUpdateError(c, err, id)
		return
	}

	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found or no changes made"})
		return
	}

	ctrl.invalidateProductCache(ctx)
	c.JSON(http.StatusOK, gin.H{"message": "Category updated successfully"})
}

// DeleteCategory handles DELETE /categories/:id (admin only), deleting a category from the hierarchy.
func (ctrl *CategoryController) DeleteCategory(c *gin.Context) {
	id := c.Param("id")
	categoryID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout)
	defer cancel()

	err = ctrl.service.DeleteCategory(ctx, categoryID)
	if err != nil {
		handleCategoryDeleteError(c, err, id)
		return
	}

	ctrl.invalidateProductCache(ctx)
	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}

// CreateBulkCategories creates multiple categories at once
func (ctrl *CategoryController) CreateBulkCategories(c *gin.Context) {
	var req services.BulkCategoryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := ctrl.validator.validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout*5) // Longer timeout for bulk
	defer cancel()

	categories, err := ctrl.service.CreateBulkCategories(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctrl.invalidateProductCache(ctx)
	c.JSON(http.StatusCreated, gin.H{
		"message":    fmt.Sprintf("Successfully created %d categories", len(categories)),
		"categories": categories,
	})
}

// Helper functions for error handling

func handleCategoryCreateError(c *gin.Context, err error) {
	if strings.Contains(err.Error(), "already exists") {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	zap.L().Error("Service failed to create category", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create category"})
}

func handleCategoryUpdateError(c *gin.Context, err error, id string) {
	if strings.Contains(err.Error(), "already exists") {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	zap.L().Error("Service failed to update category", zap.Error(err), zap.String("id", id))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update category"})
}

func handleCategoryDeleteError(c *gin.Context, err error, id string) {
	if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}
	if strings.Contains(err.Error(), "associated products") || strings.Contains(err.Error(), "has children") {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	zap.L().Error("Service failed to delete category", zap.Error(err), zap.String("id", id))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete category"})
}
