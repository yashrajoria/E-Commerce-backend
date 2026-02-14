package controllers

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
	service CategoryServiceAPI
}

func NewCategoryController(s CategoryServiceAPI) *CategoryController {
	return &CategoryController{service: s}
}

func (ctrl *CategoryController) CreateCategory(c *gin.Context) {
	var req services.CategoryCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if err := validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	category, err := ctrl.service.CreateCategory(c.Request.Context(), req)
	if err != nil {
		// Check for specific, known errors to give better feedback
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
		return
	}
	c.JSON(http.StatusCreated, category)
}

func (ctrl *CategoryController) GetCategories(c *gin.Context) {
	categoryTree, err := ctrl.service.GetCategoryTree(c.Request.Context())
	if err != nil {
		zap.L().Error("Service failed to get category tree", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch categories"})
		return
	}
	c.JSON(http.StatusOK, categoryTree)
}

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
	if err := validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	modifiedCount, err := ctrl.service.UpdateCategory(c.Request.Context(), categoryID, req)
	if err != nil {
		zap.L().Error("Service failed to update category", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update category"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found or no changes made"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category updated successfully"})
}

func (ctrl *CategoryController) DeleteCategory(c *gin.Context) {
	id := c.Param("id")
	categoryID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
		return
	}

	err = ctrl.service.DeleteCategory(c.Request.Context(), categoryID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		if strings.Contains(err.Error(), "associated products") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		zap.L().Error("Service failed to delete category", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete category"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}
