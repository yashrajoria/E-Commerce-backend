package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PresignedURLHandler handles presigned URL generation for S3 uploads
type PresignedURLHandler struct {
	productService ProductServiceAPI
	timeout        time.Duration
}

func NewPresignedURLHandler(ps ProductServiceAPI) *PresignedURLHandler {
	return &PresignedURLHandler{
		productService: ps,
		timeout:        DefaultContextTimeout,
	}
}

// GetPresignUpload returns a presigned URL for direct S3 upload by SKU
func (h *PresignedURLHandler) GetPresignUpload(c *gin.Context) {
	sku := strings.TrimSpace(c.Query("sku"))
	if sku == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SKU query parameter is required"})
		return
	}

	// Parse and validate parameters
	params, err := h.parsePresignParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate content type for images
	if !isAllowedImageContentType(params.ContentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid content type. Allowed: %v", getAllowedImageTypes()),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	uploadURL, key, publicURL, err := h.productService.GeneratePresignedUpload(
		ctx,
		sku,
		params.Filename,
		params.ContentType,
		params.Expires,
	)
	if err != nil {
		zap.L().Error("Failed to generate presigned upload", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate presigned upload"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": uploadURL,
		"method":     "PUT",
		"key":        key,
		"public_url": publicURL,
		"expires_in": params.Expires,
	})
}

// PostPresignUpload returns a presigned URL for PUT upload for a specific product ID
func (h *PresignedURLHandler) PostPresignUpload(c *gin.Context) {
	id := c.Param("id")
	productID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid UUID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	// Verify product exists
	_, err = h.productService.GetProduct(ctx, productID)
	if err != nil {
		if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Product not found"})
			return
		}
		zap.L().Error("Failed to verify product", zap.Error(err), zap.String("id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Parse parameters
	params, err := h.parsePresignParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate content type
	if !isAllowedImageContentType(params.ContentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid content type. Allowed: %v", getAllowedImageTypes()),
		})
		return
	}

	// Generate presigned URL
	url, key, err := h.generatePresignedURL(ctx, productID, params.Filename, params.Expires)
	if err != nil {
		zap.L().Error("Failed to generate presigned URL", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate presigned upload"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": url,
		"method":     "PUT",
		"key":        key,
		"expires_in": params.Expires,
	})
}

// Private helper methods

type presignParams struct {
	Filename    string
	ContentType string
	Expires     int64
}

func (h *PresignedURLHandler) parsePresignParams(c *gin.Context) (*presignParams, error) {
	filename := c.DefaultQuery("filename", "upload")
	contentType := c.DefaultQuery("content_type", "image/jpeg")

	expiresStr := c.DefaultQuery("expires", "900")
	expires, err := strconv.ParseInt(expiresStr, 10, 64)
	if err != nil || expires <= 0 {
		expires = 900
	}
	// Cap at 1 hour for security
	if expires > 3600 {
		expires = 3600
	}

	return &presignParams{
		Filename:    filename,
		ContentType: contentType,
		Expires:     expires,
	}, nil
}

func (h *PresignedURLHandler) generatePresignedURL(ctx context.Context, productID uuid.UUID, filename string, expires int64) (string, string, error) {
	cfg, err := aws_pkg.LoadAWSConfig(ctx)
	if err != nil {
		return "", "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	bucket := os.Getenv("S3_BUCKET_IMAGES")
	if bucket == "" {
		bucket = "ecommerce-product-images"
	}

	key := fmt.Sprintf("product/%s/%s", productID.String(), filename)
	url, _, err := aws_pkg.GeneratePresignedPutURL(ctx, cfg, bucket, key, expires)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return url, key, nil
}

func isAllowedImageContentType(contentType string) bool {
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}
	return allowedTypes[contentType]
}

func getAllowedImageTypes() []string {
	types := []string{"image/jpeg", "image/jpg", "image/png", "image/webp", "image/gif"}
	sort.Strings(types)
	return types
}
