package controllers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AdminProductController is a minimal controller used by admin routes and tests.
type AdminProductController struct {
	logger     *zap.Logger
	httpClient *http.Client
	baseURL    string
}

func NewAdminProductController(logger *zap.Logger, client *http.Client) *AdminProductController {
	return &AdminProductController{logger: logger, httpClient: client, baseURL: "http://product-service:8082"}
}

// bindUpdateProductBody validates an update payload allowing images-only updates.
func (apc *AdminProductController) bindUpdateProductBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, false
	}
	// Empty body -> reject
	if len(body) == 0 {
		return nil, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	// Accept only if payload has exactly one key "images" (images-only)
	if len(payload) == 1 {
		if _, ok := payload["images"]; ok {
			return body, true
		}
	}
	return nil, false
}

func (apc *AdminProductController) GetPresignUpload(c *gin.Context) {
	// Forward to downstream product service /products/presign preserving query
	url := apc.baseURL + "/products/presign"
	if q := c.Request.URL.RawQuery; q != "" {
		url += "?" + q
	}
	resp, err := apc.httpClient.Get(url)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "downstream unavailable"})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Status(resp.StatusCode)
	c.Writer.Write(data)
}

func (apc *AdminProductController) PostProductImagePresign(c *gin.Context) {
	id := c.Param("id")
	url := apc.baseURL + "/products/" + id + "/images/presign"
	if q := c.Request.URL.RawQuery; q != "" {
		url += "?" + q
	}
	req, err := http.NewRequest(http.MethodPost, url, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
		return
	}
	req.Header = c.Request.Header
	resp, err := apc.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "downstream unavailable"})
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	c.Status(resp.StatusCode)
	c.Writer.Write(data)
}

// The following methods are minimal no-ops to satisfy route bindings.
func (apc *AdminProductController) ListProducts(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (apc *AdminProductController) CreateProduct(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (apc *AdminProductController) UpdateProduct(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (apc *AdminProductController) DeleteProduct(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// Other admin controllers: minimal structs with required methods used by routes.
type AdminCategoryController struct{}

func (a *AdminCategoryController) ListCategories(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminCategoryController) CreateCategory(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminCategoryController) UpdateCategory(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminCategoryController) DeleteCategory(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func NewAdminCategoryController(logger *zap.Logger, client *http.Client) *AdminCategoryController {
	return &AdminCategoryController{}
}

type AdminUserController struct{}

func (a *AdminUserController) ListUsers(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminUserController) UpdateUserRole(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminUserController) DeleteUser(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func NewAdminUserController(logger *zap.Logger, authClient *http.Client) *AdminUserController {
	return &AdminUserController{}
}

type AdminOrderController struct{}

func (a *AdminOrderController) ListOrders(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminOrderController) GetOrderByID(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminOrderController) UpdateOrderStatus(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func NewAdminOrderController(logger *zap.Logger, orderClient *http.Client) *AdminOrderController {
	return &AdminOrderController{}
}

type AdminInventoryController struct{}

func (a *AdminInventoryController) ListInventory(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminInventoryController) UpdateInventory(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func NewAdminInventoryController(logger *zap.Logger, inventoryClient *http.Client) *AdminInventoryController {
	return &AdminInventoryController{}
}

type AdminPromotionController struct{}

func (a *AdminPromotionController) ListCoupons(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminPromotionController) CreateCoupon(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminPromotionController) UpdateCoupon(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminPromotionController) DeleteCoupon(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func NewAdminPromotionController(logger *zap.Logger, promoClient *http.Client) *AdminPromotionController {
	return &AdminPromotionController{}
}

type AdminNotificationController struct{}

func (a *AdminNotificationController) ListNotifications(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func (a *AdminNotificationController) NotificationLog(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}
func NewAdminNotificationController(logger *zap.Logger, notifClient *http.Client) *AdminNotificationController {
	return &AdminNotificationController{}
}
