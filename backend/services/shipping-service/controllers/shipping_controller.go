package controllers

import (
	"net/http"
	"shipping-service/models"
	"shipping-service/services"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ShippingController handles HTTP requests for shipping operations.
type ShippingController struct {
	shippingService services.ShippingService
}

// NewShippingController creates a new ShippingController.
func NewShippingController(svc services.ShippingService) *ShippingController {
	return &ShippingController{shippingService: svc}
}

// GetRates handles POST /shipping/rates
func (sc *ShippingController) GetRates(ctx *gin.Context) {
	var req models.ShippingRatesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	rates, svcErr := sc.shippingService.GetRates(ctx.Request.Context(), &req)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"rates": rates})
}

// CreateLabel handles POST /shipping/labels
func (sc *ShippingController) CreateLabel(ctx *gin.Context) {
	var req models.CreateLabelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	shipment, svcErr := sc.shippingService.CreateLabel(ctx.Request.Context(), &req)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"shipment": shipment})
}

// TrackShipment handles GET /shipping/track/:tracking_code
func (sc *ShippingController) TrackShipment(ctx *gin.Context) {
	trackingCode := ctx.Param("tracking_code")
	if trackingCode == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Tracking code is required"})
		return
	}

	status, svcErr := sc.shippingService.TrackShipment(ctx.Request.Context(), trackingCode)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, status)
}

// parsePaginationParams extracts and validates page/limit query params.
func parsePaginationParams(ctx *gin.Context) (int, int) {
	const maxLimit = 100
	pageInt, limitInt := 1, 10
	if p, err := strconv.Atoi(ctx.DefaultQuery("page", "1")); err == nil && p > 0 {
		pageInt = p
	}
	if l, err := strconv.Atoi(ctx.DefaultQuery("limit", "10")); err == nil && l > 0 {
		if l > maxLimit {
			l = maxLimit
		}
		limitInt = l
	}
	return pageInt, limitInt
}
