// Package controllers provides HTTP request handlers for cart operations and checkout initialization.
package controllers

import (
	"net/http"
	"strings"
	"time"

	"catalog-service/cart/database"
	"catalog-service/cart/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/yashrajoria/common/messaging"
	"go.uber.org/zap"
)

// CartController represents cart controller.
type CartController struct {
	Repo      *database.CartRepository
	SNSClient messaging.Publisher
	CartTTL   time.Duration
	Validator ProductValidator
}

// NewCartController creates and initializes a new CartController instance.
func NewCartController(repo *database.CartRepository, snsClient messaging.Publisher, cartTTL time.Duration, validator ProductValidator) *CartController {
	return &CartController{
		Repo:      repo,
		SNSClient: snsClient,
		CartTTL:   cartTTL,
		Validator: validator,
	}
}

func resolveUserID(c *gin.Context) string {
	uid := c.GetString("user_id")
	if uid != "" {
		return uid
	}
	uid = c.GetHeader("X-User-ID")
	if uid != "" {
		return uid
	}
	if v, err := c.Cookie("user_id"); err == nil && v != "" {
		return v
	}
	guestID := c.GetHeader("X-Guest-Session-ID")
	if guestID != "" {
		return "guest:" + guestID
	}
	return ""
}

// GetCart handles GET /cart, retrieving the active shopping cart and computing line item totals.
func (cc *CartController) GetCart(c *gin.Context) {
	userID := resolveUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	ctx := c.Request.Context()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		zap.L().Error("{GET CART FAILED} for user", zap.String("userID", userID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cart"})
		return
	}

	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	c.JSON(http.StatusOK, cart)
}

// AddItemsRequest specifies items to add or update in a customer's shopping cart.
type AddItemsRequest struct {
	Items []struct {
		ProductID string `json:"product_id" binding:"required,uuid"`
		Quantity  int    `json:"quantity" binding:"required,min=1,max=999"`
	} `json:"items" binding:"required,dive"`
}

const (
	maxQuantityPerProduct = 999
	maxDistinctItems      = 100
)

// AddItems handles POST /cart/items, adding new product items or incrementing quantities in the shopping cart.
func (cc *CartController) AddItems(c *gin.Context) {
	userID := resolveUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	var req AddItemsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cart"})
		return
	}

	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	// Update cart items: increment quantities if product exists, else add new
	for _, newItem := range req.Items {
		found := false
		for i, existing := range cart.Items {
			if existing.ProductID == newItem.ProductID {
				cart.Items[i].Quantity += newItem.Quantity
				if cart.Items[i].Quantity > maxQuantityPerProduct {
					c.JSON(http.StatusBadRequest, gin.H{"error": "quantity exceeds per-product limit of 999"})
					return
				}
				found = true
				break
			}
		}
		if !found {
			if len(cart.Items) >= maxDistinctItems {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cart exceeds distinct-item limit of 100"})
				return
			}
			cart.Items = append(cart.Items, models.CartItem{
				ProductID: newItem.ProductID,
				Quantity:  newItem.Quantity,
			})
		}
	}

	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save cart"})
		return
	}

	c.JSON(http.StatusOK, cart)
}

// RemoveItem handles DELETE /cart/items/:id, removing a product line item from the cart.
func (cc *CartController) RemoveItem(c *gin.Context) {
	userID := resolveUserID(c)
	productID := c.Param("product_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	ctx := c.Request.Context()

	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		zap.L().Error("[RemoveItem] Failed to get cart", zap.String("userID", userID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get cart"})
		return
	}
	if cart == nil {
		zap.L().Warn("[RemoveItem] Cart not found", zap.String("userID", userID))

		c.JSON(http.StatusNotFound, gin.H{"error": "cart not found"})
		return
	}

	newItems := []models.CartItem{}
	for _, item := range cart.Items {
		if item.ProductID != productID {
			newItems = append(newItems, item)
		}
	}
	cart.Items = newItems

	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		zap.L().Error("[RemoveItem] Failed to update cart", zap.String("userID", userID), zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update cart"})
		return
	}

	c.JSON(http.StatusOK, cart)
}

// ClearCart handles DELETE /cart, emptying all items from the shopping cart session.
func (cc *CartController) ClearCart(c *gin.Context) {
	userID := resolveUserID(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not authorized"})
		return
	}

	ctx := c.Request.Context()

	err := cc.Repo.DeleteCart(ctx, userID)
	if err != nil {
		zap.L().Error("[ClearCart] Failed to clear cart", zap.String("userID", userID), zap.Error(err))

		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cart"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cart cleared"})
}

// MergeCartRequest defines the request payload for merge cart operations.
type MergeCartRequest struct {
	GuestSessionID string `json:"guest_session_id" binding:"required"`
}

// MergeCart merges an anonymous guest cart into the authenticated user's cart
func (cc *CartController) MergeCart(c *gin.Context) {
	authUserID := c.GetHeader("X-User-ID")
	if authUserID == "" {
		authUserID = c.GetString("user_id")
	}
	if authUserID == "" || strings.HasPrefix(authUserID, "guest:") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authenticated user required to merge cart"})
		return
	}

	var req MergeCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid merge request", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()
	guestUserID := req.GuestSessionID
	if !strings.HasPrefix(guestUserID, "guest:") {
		guestUserID = "guest:" + guestUserID
	}

	guestCart, err := cc.Repo.GetCart(ctx, guestUserID)
	if err != nil || guestCart == nil || len(guestCart.Items) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "merged": 0})
		return
	}

	userCart, err := cc.Repo.GetCart(ctx, authUserID)
	if err != nil || userCart == nil {
		userCart = &models.Cart{UserID: authUserID, Items: []models.CartItem{}}
	}

	mergedCount := 0
	for _, gItem := range guestCart.Items {
		found := false
		for i, uItem := range userCart.Items {
			if uItem.ProductID == gItem.ProductID {
				userCart.Items[i].Quantity += gItem.Quantity
				found = true
				break
			}
		}
		if !found {
			userCart.Items = append(userCart.Items, gItem)
		}
		mergedCount++
	}

	_ = cc.Repo.SaveCart(ctx, userCart)
	_ = cc.Repo.DeleteCart(ctx, guestUserID)

	c.JSON(http.StatusOK, gin.H{"status": "ok", "merged": mergedCount, "cart": userCart})
}

// ClearUserCart empties a user's cart on behalf of order-service once their payment
// succeeded. Mesh-only (internalauth + gateway blocks /internal/ paths).
func (cc *CartController) ClearUserCart(c *gin.Context) {
	userID := c.Param("user_id")
	if _, err := uuid.Parse(userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	if err := cc.Repo.DeleteCart(c.Request.Context(), userID); err != nil {
		zap.L().Error("[ClearUserCart] Failed to clear cart", zap.String("userID", userID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear cart"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "cart cleared"})
}

// ApplyCoupon validates and associates a promotional coupon code with the user's active shopping cart.
func (cc *CartController) ApplyCoupon(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		if v, err := c.Cookie("user_id"); err == nil && v != "" {
			userID = v
		}
	}

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		CouponCode string `json:"coupon_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()
	cart, err := cc.Repo.GetCart(ctx, userID)
	if err != nil {
		zap.L().Error("failed to fetch cart", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if cart == nil {
		cart = &models.Cart{
			UserID: userID,
			Items:  []models.CartItem{},
		}
	}

	cart.CouponCode = req.CouponCode
	if err := cc.Repo.SaveCart(ctx, cart); err != nil {
		zap.L().Error("failed to update cart with coupon", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply coupon"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "coupon applied successfully",
		"coupon_code": req.CouponCode,
	})
}
