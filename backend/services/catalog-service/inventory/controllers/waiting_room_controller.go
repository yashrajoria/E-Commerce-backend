package controllers

import (
	"net/http"

	"catalog-service/inventory/models"
	"catalog-service/inventory/services"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// WaitingRoomController handles HTTP traffic for the flash sale waiting room
type WaitingRoomController struct {
	service services.WaitingRoomService
}

func NewWaitingRoomController(service services.WaitingRoomService) *WaitingRoomController {
	return &WaitingRoomController{service: service}
}

// Configure sets up the waiting room for a product (admin only)
// POST /inventory/flash-sale/configure
func (w *WaitingRoomController) Configure(c *gin.Context) {
	var req models.ConfigureFlashSaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	userID := c.GetHeader("X-User-ID")
	zap.L().Info("[AUDIT] admin configure flash sale waiting room",
		zap.String("user_id", userID),
		zap.String("product_id", req.ProductID),
		zap.Bool("active", req.Active),
		zap.Int("slots", req.TotalSlots),
	)

	if err := w.service.Configure(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to configure flash sale", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Flash sale waiting room configured successfully",
		"product_id": req.ProductID,
		"active":     req.Active,
		"slots":      req.TotalSlots,
	})
}

// Enter attempts to acquire a lease or puts user in the waiting room
// POST /inventory/flash-sale/enter
func (w *WaitingRoomController) Enter(c *gin.Context) {
	var req models.EnterQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	// Fallback to X-User-ID header if user_id is empty in body
	if req.UserID == "" {
		req.UserID = c.GetHeader("X-User-ID")
	}
	if req.UserID == "" {
		req.UserID = "anon_" + c.ClientIP()
	}

	resp, err := w.service.EnterQueue(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enter waiting room", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Status checks position in queue or current lease status
// GET /inventory/flash-sale/status?product_id=XYZ&user_id=ABC
func (w *WaitingRoomController) Status(c *gin.Context) {
	productID := c.Query("product_id")
	userID := c.Query("user_id")
	if userID == "" {
		userID = c.GetHeader("X-User-ID")
	}
	if userID == "" {
		userID = "anon_" + c.ClientIP()
	}

	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "product_id is required"})
		return
	}

	resp, err := w.service.GetStatus(c.Request.Context(), productID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch status", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Claim validates and consumes a lease token during checkout
// POST /inventory/flash-sale/claim
func (w *WaitingRoomController) Claim(c *gin.Context) {
	var req models.ClaimLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if req.UserID == "" {
		req.UserID = c.GetHeader("X-User-ID")
	}

	ok, err := w.service.ClaimLease(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to claim lease", "details": err.Error()})
		return
	}

	if !ok {
		c.JSON(http.StatusConflict, gin.H{"error": "Invalid, expired, or unassigned lease token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lease successfully claimed", "product_id": req.ProductID})
}

// Release returns a lease back to the pool
// POST /inventory/flash-sale/release
func (w *WaitingRoomController) Release(c *gin.Context) {
	var req models.ReleaseLeaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	if req.UserID == "" {
		req.UserID = c.GetHeader("X-User-ID")
	}

	if err := w.service.ReleaseLease(c.Request.Context(), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to release lease", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Lease released", "product_id": req.ProductID})
}

// Reset clears the waiting room for a product (admin only)
// POST /inventory/flash-sale/reset
func (w *WaitingRoomController) Reset(c *gin.Context) {
	productID := c.Query("product_id")
	if productID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "product_id is required"})
		return
	}

	userID := c.GetHeader("X-User-ID")
	zap.L().Info("[AUDIT] admin reset flash sale waiting room",
		zap.String("user_id", userID),
		zap.String("product_id", productID),
	)

	if err := w.service.Reset(c.Request.Context(), productID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset waiting room", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Waiting room reset successfully", "product_id": productID})
}
