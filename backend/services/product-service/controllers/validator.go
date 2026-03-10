package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"product-service/services"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// Validation constants
const (
	MaxPageSize   = 100
	MaxPageNumber = 1000000
	MaxUploadSize = 50 * 1024 * 1024 // 50MB
)

// Allowed file types
var (
	allowedCSVExtensions = map[string]bool{
		".csv": true,
		".txt": true,
	}

	allowedImageTypes = map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}
)

// CreateProductRequest defines the expected structure for creating a product
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

// ProductFilters holds all filter parameters
type ProductFilters struct {
	IsFeatured       string
	CategoryKey      string
	CategoryIDs      []uuid.UUID
	SortParam        string
	MinPrice         *float64
	MaxPrice         *float64
	IsFeaturedParsed *bool
	Brand            string
	InStockParsed    *bool
}

// RequestValidator handles all input validation
type RequestValidator struct {
	validate *validator.Validate
}

func NewRequestValidator() *RequestValidator {
	return &RequestValidator{
		validate: validator.New(),
	}
}

// ParsePagination validates and parses pagination parameters
func (rv *RequestValidator) ParsePagination(c *gin.Context) (int, int, error) {
	pageStr := c.DefaultQuery("page", "1")
	perPageStr := c.DefaultQuery("perPage", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		return 0, 0, errors.New("invalid page number")
	}
	if page > MaxPageNumber {
		page = MaxPageNumber
	}

	perPage, err := strconv.Atoi(perPageStr)
	if err != nil || perPage < 1 {
		return 0, 0, errors.New("invalid page size")
	}
	if perPage > MaxPageSize {
		perPage = MaxPageSize
	}

	return page, perPage, nil
}

// ParseFilters validates and parses all filter parameters
func (rv *RequestValidator) ParseFilters(c *gin.Context) (*ProductFilters, error) {
	filters := &ProductFilters{}

	// Parse is_featured
	if err := rv.parseIsFeatured(c, filters); err != nil {
		return nil, err
	}

	// Parse category IDs
	if err := rv.parseCategoryIDs(c, filters); err != nil {
		return nil, err
	}

	// Parse sort parameter
	if err := rv.parseSortParam(c, filters); err != nil {
		return nil, err
	}

	// Parse price range
	if err := rv.parsePriceRange(c, filters); err != nil {
		return nil, err
	}

	// Parse brand
	if err := rv.parseBrand(c, filters); err != nil {
		return nil, err
	}

	// Parse in_stock
	if err := rv.parseInStock(c, filters); err != nil {
		return nil, err
	}

	return filters, nil
}

func (rv *RequestValidator) parseBrand(c *gin.Context, filters *ProductFilters) error {
	brand := strings.TrimSpace(c.Query("brand"))
	if brand != "" {
		filters.Brand = brand
	}
	return nil
}

func (rv *RequestValidator) parseInStock(c *gin.Context, filters *ProductFilters) error {
	inStockStr := strings.TrimSpace(c.Query("in_stock"))
	if inStockStr != "" {
		v, err := strconv.ParseBool(inStockStr)
		if err != nil {
			return errors.New("invalid boolean value for 'in_stock'")
		}
		filters.InStockParsed = &v
	}
	return nil
}

// ParseCreateProductRequest validates and parses product creation request
func (rv *RequestValidator) ParseCreateProductRequest(c *gin.Context) (services.ProductCreateRequest, []*multipart.FileHeader, error) {
	var req CreateProductRequest
	if err := c.ShouldBind(&req); err != nil {
		return services.ProductCreateRequest{}, nil, fmt.Errorf("invalid form data: %w", err)
	}

	if err := rv.validate.Struct(&req); err != nil {
		return services.ProductCreateRequest{}, nil, fmt.Errorf("validation failed: %w", err)
	}

	// Parse categories
	var categoryNames []string
	if err := json.Unmarshal([]byte(req.Categories), &categoryNames); err != nil {
		return services.ProductCreateRequest{}, nil, errors.New("invalid category format, must be a JSON string array")
	}

	// Get images
	form, err := c.MultipartForm()
	if err != nil {
		return services.ProductCreateRequest{}, nil, errors.New("expected multipart form data")
	}

	images := form.File["images"]
	if len(images) == 0 {
		return services.ProductCreateRequest{}, nil, errors.New("at least one image is required")
	}

	// Validate image file types
	for _, img := range images {
		if !rv.IsValidImageType(img) {
			return services.ProductCreateRequest{}, nil, fmt.Errorf("invalid image type for file %s. Allowed: jpeg, jpg, png, webp, gif", img.Filename)
		}
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

	return serviceReq, images, nil
}

// IsValidImageType checks if the file is a valid image
func (rv *RequestValidator) IsValidImageType(file *multipart.FileHeader) bool {
	// Check by content type
	if allowedImageTypes[file.Header.Get("Content-Type")] {
		return true
	}

	// Fallback: check by extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}

	return false
}

// IsValidCSVFile checks if the file is a valid CSV
func (rv *RequestValidator) IsValidCSVFile(file *multipart.FileHeader) bool {
	// Check content type
	contentType := file.Header.Get("Content-Type")
	if contentType == "text/csv" || contentType == "application/csv" || contentType == "text/plain" {
		return true
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	return allowedCSVExtensions[ext]
}

// ValidateFileSize checks if file size is within limits
func (rv *RequestValidator) ValidateFileSize(file *multipart.FileHeader) error {
	if file.Size > MaxUploadSize {
		return fmt.Errorf("file too large (max %dMB)", MaxUploadSize/(1024*1024))
	}
	return nil
}

// Private helper methods

func (rv *RequestValidator) parseIsFeatured(c *gin.Context, filters *ProductFilters) error {
	isFeaturedStr := c.Query("is_featured")
	if isFeaturedStr != "" {
		isFeatured, err := strconv.ParseBool(isFeaturedStr)
		if err != nil {
			return errors.New("invalid boolean value for 'is_featured'")
		}
		filters.IsFeaturedParsed = &isFeatured
	}
	filters.IsFeatured = strings.ToLower(strings.TrimSpace(isFeaturedStr))
	return nil
}

func (rv *RequestValidator) parseCategoryIDs(c *gin.Context, filters *ProductFilters) error {
	categoryIDsParam := c.Query("categoryId")
	if categoryIDsParam == "" {
		return nil
	}

	rawIDs := strings.Split(categoryIDsParam, ",")
	var categoryIDStrings []string

	for _, rawID := range rawIDs {
		trimmed := strings.TrimSpace(rawID)
		if trimmed == "" {
			continue
		}

		categoryUUID, err := uuid.Parse(trimmed)
		if err != nil {
			return errors.New("invalid category ID format")
		}

		filters.CategoryIDs = append(filters.CategoryIDs, categoryUUID)
		categoryIDStrings = append(categoryIDStrings, categoryUUID.String())
	}

	if len(filters.CategoryIDs) == 0 {
		return errors.New("invalid category ID format")
	}

	sort.Strings(categoryIDStrings)
	filters.CategoryKey = strings.Join(categoryIDStrings, ",")
	return nil
}

func (rv *RequestValidator) parseSortParam(c *gin.Context, filters *ProductFilters) error {
	sortParam := strings.TrimSpace(c.Query("sort"))
	if sortParam != "" && !isSupportedSort(sortParam) {
		return errors.New("invalid sort value")
	}
	filters.SortParam = strings.ToLower(sortParam)
	return nil
}

func (rv *RequestValidator) parsePriceRange(c *gin.Context, filters *ProductFilters) error {
	// Parse min price
	minPriceStr := strings.TrimSpace(c.Query("minPrice"))
	if minPriceStr != "" {
		parsed, err := strconv.ParseFloat(minPriceStr, 64)
		if err != nil {
			return errors.New("invalid minPrice value")
		}
		filters.MinPrice = &parsed
	}

	// Parse max price
	maxPriceStr := strings.TrimSpace(c.Query("maxPrice"))
	if maxPriceStr != "" {
		parsed, err := strconv.ParseFloat(maxPriceStr, 64)
		if err != nil {
			return errors.New("invalid maxPrice value")
		}
		filters.MaxPrice = &parsed
	}

	// Validate range
	if filters.MinPrice != nil && filters.MaxPrice != nil && *filters.MinPrice > *filters.MaxPrice {
		return errors.New("minPrice must be less than or equal to maxPrice")
	}

	return nil
}

func isSupportedSort(sortParam string) bool {
	switch sortParam {
	case "price_asc", "price_desc", "created_at_asc", "created_at_desc", "name_asc", "name_desc":
		return true
	default:
		return false
	}
}
