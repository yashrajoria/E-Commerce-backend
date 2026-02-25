package controllers

import (
	"net/http"
	"promotion-service/models"
	"promotion-service/services"
	"strconv"

	"github.com/gin-gonic/gin"
)

// CouponController handles HTTP requests for coupon operations.
type CouponController struct {
	couponService services.CouponService
}

// NewCouponController creates a new CouponController.
func NewCouponController(couponService services.CouponService) *CouponController {
	return &CouponController{couponService: couponService}
}

// CreateCoupon handles POST /coupons (admin only).
func (cc *CouponController) CreateCoupon(ctx *gin.Context) {
	var req models.CreateCouponRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	coupon, svcErr := cc.couponService.CreateCoupon(ctx.Request.Context(), &req)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{"coupon": coupon})
}

// ValidateCoupon handles POST /coupons/validate (called by cart-service).
func (cc *CouponController) ValidateCoupon(ctx *gin.Context) {
	var req models.ValidateCouponRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}

	resp, svcErr := cc.couponService.ValidateCoupon(ctx.Request.Context(), &req)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

// GetCoupon handles GET /coupons/:code.
func (cc *CouponController) GetCoupon(ctx *gin.Context) {
	code := ctx.Param("code")
	if code == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Coupon code is required"})
		return
	}

	coupon, svcErr := cc.couponService.GetCoupon(ctx.Request.Context(), code)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"coupon": coupon})
}

// DeactivateCoupon handles DELETE /coupons/:code (admin only).
func (cc *CouponController) DeactivateCoupon(ctx *gin.Context) {
	code := ctx.Param("code")
	if code == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Coupon code is required"})
		return
	}

	svcErr := cc.couponService.DeactivateCoupon(ctx.Request.Context(), code)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Coupon deactivated"})
}

// ListCoupons handles GET /coupons (admin only).
func (cc *CouponController) ListCoupons(ctx *gin.Context) {
	page, limit := parsePaginationParams(ctx)

	coupons, total, svcErr := cc.couponService.ListCoupons(ctx.Request.Context(), page, limit)
	if svcErr != nil {
		ctx.JSON(svcErr.StatusCode, gin.H{"error": svcErr.Message})
		return
	}

	totalPages := int64(0)
	if limit > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"coupons": coupons,
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
			"has_more":    total > int64(page*limit),
		},
	})
}

// parsePaginationParams extracts and validates pagination parameters.
func parsePaginationParams(ctx *gin.Context) (int, int) {
	const MaxLimit = 100
	const DefaultPage = 1
	const DefaultLimit = 10

	page := ctx.DefaultQuery("page", "1")
	limit := ctx.DefaultQuery("limit", "10")

	pageInt := DefaultPage
	limitInt := DefaultLimit

	if p, err := strconv.Atoi(page); err == nil && p > 0 {
		pageInt = p
	}

	if l, err := strconv.Atoi(limit); err == nil && l > 0 {
		limitInt = l
		if limitInt > MaxLimit {
			limitInt = MaxLimit
		}
	}

	return pageInt, limitInt
}
