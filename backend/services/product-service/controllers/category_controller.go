package controllers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"product-service/models"
	"product-service/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CategoryServiceAPI defines the interface for category service operations
type CategoryServiceAPI interface {
	CreateCategory(ctx context.Context, req services.CategoryCreateRequest) (*models.Category, error)
	GetCategoryTree(ctx context.Context) ([]*models.Category, error)
	UpdateCategory(ctx context.Context, id uuid.UUID, req services.CategoryCreateRequest) (int64, error)
	DeleteCategory(ctx context.Context, id uuid.UUID) error
	GetCategory(ctx context.Context, id uuid.UUID) (*models.Category, error)
}

type CategoryController struct {
	service   CategoryServiceAPI
	validator *RequestValidator
	timeout   time.Duration
}

func NewCategoryController(s CategoryServiceAPI) *CategoryController {
	return &CategoryController{
		service:   s,
		validator: NewRequestValidator(),
		timeout:   DefaultContextTimeout,
	}
}

// WithTimeout sets the context timeout for operations
func (ctrl *CategoryController) WithTimeout(timeout time.Duration) *CategoryController {
	ctrl.timeout = timeout
	return ctrl
}

// CreateCategory creates a new category
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

	c.JSON(http.StatusCreated, category)
}

// GetCategories retrieves the category tree
func (ctrl *CategoryController) GetCategories(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), ctrl.timeout)
	defer cancel()

	categoryTree, err := ctrl.service.GetCategoryTree(ctx)
	if err != nil {
		zap.L().Error("Service failed to get category tree", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}

	c.JSON(http.StatusOK, categoryTree)
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

// UpdateCategory updates an existing category
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

	c.JSON(http.StatusOK, gin.H{"message": "Category updated successfully"})
}

// DeleteCategory deletes a category
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

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
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
