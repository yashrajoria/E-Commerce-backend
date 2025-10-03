package controllers

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/yashrajoria/product-service/services"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// Use a single instance of Validate, it caches struct info
var validate = validator.New()

// CreateProductRequest defines the expected structure for creating a product via multipart-form.
type CreateProductRequest struct {
	Name        string  `form:"name" validate:"required"`
	Description string  `form:"description" validate:"required"`
	Brand       string  `form:"brand" validate:"required"`
	SKU         string  `form:"sku" validate:"required"`
	Price       float64 `form:"price" validate:"required,gt=0"`
	Quantity    int     `form:"quantity" validate:"required,gte=0"`
	IsFeatured  bool    `form:"is_featured"`
	Categories  string  `form:"category" validate:"required"` // JSON string array
}

type ProductController struct {
	productService *services.ProductService
}

func NewProductController(ps *services.ProductService) *ProductController {
	return &ProductController{
		productService: ps,
	}
}

func (ctrl *ProductController) GetProductByID(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	product, err := ctrl.productService.GetProduct(c.Request.Context(), productID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		zap.L().Error("Service failed to get product", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}
	c.JSON(http.StatusOK, product)
}

func (ctrl *ProductController) GetProducts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("perPage", "10"))
	if perPage <= 0 {
		perPage = 10
	}

	params := services.ListProductsParams{
		Page:    page,
		PerPage: perPage,
	}

	if isFeaturedStr := c.Query("is_featured"); isFeaturedStr != "" {
		isFeatured, err := strconv.ParseBool(isFeaturedStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid boolean value for 'is_featured'"})
			return
		}
		params.IsFeatured = &isFeatured
	}

	if categoryIDStr := c.Param("categoryId"); categoryIDStr != "" {
		categoryUUID, err := uuid.Parse(categoryIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID format"})
			return
		}
		params.CategoryID = categoryUUID
	}

	products, total, err := ctrl.productService.ListProducts(c.Request.Context(), params)
	if err != nil {
		zap.L().Error("Service failed to list products", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch products"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"meta": gin.H{
			"page":       page,
			"perPage":    perPage,
			"total":      total,
			"totalPages": totalPages,
		},
	})
}

func (ctrl *ProductController) CreateProduct(c *gin.Context) {
	var req CreateProductRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data", "details": err.Error()})
		return
	}

	if err := validate.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Validation failed", "details": err.Error()})
		return
	}

	var categoryNames []string
	if err := json.Unmarshal([]byte(req.Categories), &categoryNames); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category format, must be a JSON string array"})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Expected multipart form data"})
		return
	}
	images := form.File["images"]
	if len(images) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one image is required"})
		return
	}

	serviceReq := services.ProductCreateRequest{
		Name:        req.Name,
		Description: req.Description,
		Brand:       req.Brand,
		SKU:         req.SKU,
		Price:       req.Price,
		Quantity:    req.Quantity,
		IsFeatured:  req.IsFeatured,
		Categories:  categoryNames,
	}

	product, err := ctrl.productService.CreateProduct(c.Request.Context(), serviceReq, images)
	if err != nil {
		zap.L().Error("Service failed to create product", zap.Error(err))
		// You can add more specific error checks here (e.g., for duplicate SKU)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})
		return
	}

	c.JSON(http.StatusCreated, product)
}

func (ctrl *ProductController) UpdateProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	var updates bson.M
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	modifiedCount, err := ctrl.productService.UpdateProduct(c.Request.Context(), productID, updates)
	if err != nil {
		zap.L().Error("Service failed to update product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update product"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found or no changes made"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product updated successfully"})
}

func (ctrl *ProductController) DeleteProduct(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	modifiedCount, err := ctrl.productService.DeleteProduct(c.Request.Context(), productID)
	if err != nil {
		zap.L().Error("Service failed to delete product", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete product"})
		return
	}
	if modifiedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Product deleted successfully"})
}

func (ctrl *ProductController) CreateBulkProducts(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File upload required in 'file' field"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
		return
	}
	defer src.Close()

	inserted, errorsList, err := ctrl.productService.CreateBulkProducts(c.Request.Context(), src)
	if err != nil {
		zap.L().Error("Service failed during bulk create", zap.Error(err))
		// Check for duplicate key errors specifically
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "Bulk insert failed due to duplicate SKU. Please ensure all SKUs are unique."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An unexpected error occurred during bulk import"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "Bulk import process completed.",
		"inserted_count": inserted,
		"errors_count":   len(errorsList),
		"errors":         errorsList,
	})
}

func (ctrl *ProductController) GetProductByIDInternal(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	productDTO, err := ctrl.productService.GetProductInternal(c.Request.Context(), productID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		zap.L().Error("Service failed to get internal product", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, productDTO)
}
