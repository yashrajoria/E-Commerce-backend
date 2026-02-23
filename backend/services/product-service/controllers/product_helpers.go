package controllers

import (
	"errors"
	"strings"

	"product-service/models"
	"product-service/services"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleServiceError maps common service errors to HTTP responses.
func handleServiceError(c *gin.Context, err error, notFoundMsg string) {
	if errorsIsNotFound(err) || strings.Contains(err.Error(), "not found") {
		c.JSON(404, gin.H{"error": notFoundMsg})
		return
	}
	zap.L().Error("Service error", zap.Error(err))
	c.JSON(500, gin.H{"error": "Internal server error"})
}

// handleCreateError centralizes create error handling and logging
func handleCreateError(c *gin.Context, err error) {
	zap.L().Error("Service failed to create product", zap.Error(err))
	if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "already exists") {
		c.JSON(409, gin.H{"error": "Product with this SKU already exists"})
		return
	}
	c.JSON(500, gin.H{"error": "Failed to create product"})
}

// buildServiceParams converts controller filters+pagination into service layer params
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

// buildProductListResponse builds a simple paginated response map
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

// errorsIsNotFound is a small helper to avoid importing errors package here
func errorsIsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
