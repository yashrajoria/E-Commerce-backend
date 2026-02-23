package controllers

import (
	"context"
	"mime/multipart"
	"time"

	"product-service/models"
	"product-service/services"

	"github.com/google/uuid"
)

// Config holds controller configuration
type Config struct {
	CacheTTL       time.Duration
	ContextTimeout time.Duration
}

// Default configuration values
const (
	DefaultCacheTTL       = 10 * time.Minute
	DefaultContextTimeout = 30 * time.Second
)

// ProductServiceAPI defines the interface for product service operations
type ProductServiceAPI interface {
	GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error)
	ListProducts(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error)
	CreateProduct(ctx context.Context, req services.ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error)
	UpdateProduct(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (int64, error)
	DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error)
	// BulkDeleteProducts supports deleting multiple products by IDs, by category IDs, or all products
	BulkDeleteProducts(ctx context.Context, req services.BulkDeleteRequest) (int64, error)
	GetProductInternal(ctx context.Context, id uuid.UUID) (*services.ProductInternalDTO, error)
	ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error)
	ProcessBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportResult, error)
	GeneratePresignedUpload(ctx context.Context, sku, filename, contentType string, expiresSeconds int64) (string, string, string, error)
}
