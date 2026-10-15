// Package controllers provides HTTP endpoints for inventory levels and waiting room queues.
package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"catalog-service/inventory/models"
	"catalog-service/inventory/repository"
	"catalog-service/inventory/services"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// InventoryController handles HTTP requests for inventory
type InventoryController struct {
	service *services.InventoryService
}

// NewInventoryController creates a new InventoryController
func NewInventoryController(service *services.InventoryService) *InventoryController {
	return &InventoryController{service: service}
}

// GetStock handles GET /inventory/:productId, returning real-time available and reserved stock counts.
func (ic *InventoryController) GetStock(c *gin.Context) {
	productID := c.Param("productId")
	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing product ID"})
		return
	}

	inv, err := ic.service.GetStock(c.Request.Context(), productID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Inventory not found for product"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch inventory"})
		return
	}

	c.JSON(http.StatusOK, inv)
}

// SetStock initializes or overwrites inventory for a product
// POST /inventory
func (ic *InventoryController) SetStock(c *gin.Context) {
	var req models.SetStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	userID := c.GetHeader("X-User-ID")
	zap.L().Info("[AUDIT] admin set stock",
		zap.String("user_id", userID),
		zap.String("product_id", req.ProductID),
		zap.Int("stock", req.Available),
	)

	inv, err := ic.service.SetStock(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set stock"})
		return
	}

	c.JSON(http.StatusCreated, inv)
}

// UpdateStock partially updates inventory for a product
// PUT /inventory/:productId
func (ic *InventoryController) UpdateStock(c *gin.Context) {
	productID := c.Param("productId")
	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing product ID"})
		return
	}

	var req models.UpdateStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	userID := c.GetHeader("X-User-ID")
	zap.L().Info("[AUDIT] admin update stock",
		zap.String("user_id", userID),
		zap.String("product_id", productID),
		zap.Any("updates", req),
	)

	inv, err := ic.service.UpdateStock(c.Request.Context(), productID, &req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Inventory not found for product"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update stock"})
		return
	}

	c.JSON(http.StatusOK, inv)
}

// ReserveStock handles POST /inventory/reserve, placing temporary holds on inventory items during checkout.
func (ic *InventoryController) ReserveStock(c *gin.Context) {
	var req models.ReserveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	results, err := ic.service.ReserveStock(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Stock reserved successfully",
		"order_id": req.OrderID,
		"results":  results,
	})
}

// ReleaseStock handles POST /inventory/release, releasing reserved inventory back to available stock.
func (ic *InventoryController) ReleaseStock(c *gin.Context) {
	var req models.ReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if err := ic.service.ReleaseStock(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to release stock"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Stock released successfully",
		"order_id": req.OrderID,
	})
}

// ConfirmStock handles POST /inventory/confirm, committing reserved inventory deductions upon completed payment.
func (ic *InventoryController) ConfirmStock(c *gin.Context) {
	var req models.ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if err := ic.service.ConfirmStock(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to confirm stock"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Stock confirmed successfully",
		"order_id": req.OrderID,
	})
}

// RestockStock returns confirmed stock to available (paid order cancelled)
// POST /inventory/restock
func (ic *InventoryController) RestockStock(c *gin.Context) {
	var req models.RestockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if err := ic.service.RestockStock(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restock"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Stock restocked successfully",
		"order_id": req.OrderID,
	})
}

// CheckStock checks stock availability for multiple items
// POST /inventory/check
func (ic *InventoryController) CheckStock(c *gin.Context) {
	var req struct {
		Items []models.ReserveItem `json:"items" binding:"required,dive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	results, err := ic.service.CheckStock(c.Request.Context(), req.Items)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check stock"})
		return
	}

	allSufficient := true
	for _, r := range results {
		if !r.IsSufficient {
			allSufficient = false
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"all_sufficient": allSufficient,
		"results":        results,
	})
}

// ListStock returns all inventory items with pagination
// GET /inventory
func (ic *InventoryController) ListStock(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page := 1
	pageSize := 20
	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(pageSizeStr); err == nil && v > 0 && v <= 100 {
		pageSize = v
	}

	items, total, err := ic.service.ListAllStock(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list inventory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inventory": items,
		"meta": gin.H{
			"page":        page,
			"page_size":   pageSize,
			"count":       len(items),
			"total":       total,
			"total_pages": (total + int64(pageSize) - 1) / int64(pageSize),
			"has_more":    int64(page*pageSize) < total,
		},
	})
}
