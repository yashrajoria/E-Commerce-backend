package controllers

import (
	"net/http"
	"shipping-service/models"
	"shipping-service/services"

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
