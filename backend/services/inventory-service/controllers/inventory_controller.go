package controllers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/inventory-service/models"
	"github.com/yashrajoria/inventory-service/repository"
	"github.com/yashrajoria/inventory-service/services"
)

// InventoryController handles HTTP requests for inventory
type InventoryController struct {
	service *services.InventoryService
}

// NewInventoryController creates a new InventoryController
func NewInventoryController(service *services.InventoryService) *InventoryController {
	return &InventoryController{service: service}
}

// GetStock returns the inventory for a product
// GET /inventory/:productId
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

// ReserveStock reserves inventory for order items
// POST /inventory/reserve
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

// ReleaseStock releases reserved stock
// POST /inventory/release
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

// ConfirmStock confirms reserved stock (payment succeeded)
// POST /inventory/confirm
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
