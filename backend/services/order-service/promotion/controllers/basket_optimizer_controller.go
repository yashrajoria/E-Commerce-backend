package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"order-service/promotion/models"
	"order-service/promotion/services"
)

type BasketOptimizerController struct {
	service services.BasketOptimizerService
}

func NewBasketOptimizerController(service services.BasketOptimizerService) *BasketOptimizerController {
	return &BasketOptimizerController{service: service}
}

// Optimize calculates weight headroom, shipping estimates, and dynamic bundling incentives.
// POST /promotions/optimize-basket
func (ctrl *BasketOptimizerController) Optimize(c *gin.Context) {
	var req models.OptimizeBasketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	resp, svcErr := ctrl.service.Optimize(c.Request.Context(), &req)
	if svcErr != nil {
		c.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	c.JSON(http.StatusOK, resp)
}
