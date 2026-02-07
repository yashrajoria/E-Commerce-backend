package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"product-service/models"
	"product-service/services"

	"mime/multipart"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type noopProductService struct{}

func (n *noopProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return nil, nil
}
func (n *noopProductService) ListProducts(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error) {
	return nil, 0, nil
}
func (n *noopProductService) CreateProduct(ctx context.Context, req services.ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error) {
	return nil, nil
}
func (n *noopProductService) UpdateProduct(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (int64, error) {
	return 0, nil
}
func (n *noopProductService) DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error) {
	return 0, nil
}
func (n *noopProductService) GetProductInternal(ctx context.Context, id uuid.UUID) (*services.ProductInternalDTO, error) {
	return nil, nil
}
func (n *noopProductService) ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error) {
	return nil, nil
}
func (n *noopProductService) ProcessBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportResult, error) {
	return nil, nil
}
func (n *noopProductService) GeneratePresignedUpload(ctx context.Context, sku, filename, contentType string, expiresSeconds int64) (string, string, string, error) {
	return "", "", "", nil
}

func TestPostPresignUpload_InvalidUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	ctrl := NewProductController(&noopProductService{}, nil)
	r.POST("/products/:id/images/presign", ctrl.PostPresignUpload)

	req := httptest.NewRequest(http.MethodPost, "/products/not-a-uuid/images/presign", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid uuid, got %d", w.Code)
	}
}
