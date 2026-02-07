package controllers

import (
	"context"
	"errors"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"product-service/models"
	"product-service/services"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type fakeProductService struct {
	lastParams         services.ListProductsParams
	listProductsCalled int
	listProductsFn     func(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error)
}

func (f *fakeProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return nil, nil
}

func (f *fakeProductService) ListProducts(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error) {
	f.listProductsCalled++
	f.lastParams = params
	if f.listProductsFn != nil {
		return f.listProductsFn(ctx, params)
	}
	return []*models.Product{}, 0, nil
}

func (f *fakeProductService) CreateProduct(ctx context.Context, req services.ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error) {
	return nil, nil
}

func (f *fakeProductService) UpdateProduct(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (int64, error) {
	return 0, nil
}

func (f *fakeProductService) DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeProductService) GetProductInternal(ctx context.Context, id uuid.UUID) (*services.ProductInternalDTO, error) {
	return nil, nil
}

func (f *fakeProductService) ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error) {
	return nil, nil
}

func (f *fakeProductService) ProcessBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportResult, error) {
	return nil, nil
}

func (f *fakeProductService) GeneratePresignedUpload(ctx context.Context, sku, filename, contentType string, expiresSeconds int64) (string, string, string, error) {
	return "", "", "", nil
}

func newTestRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: "localhost:0",
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, errors.New("redis disabled in tests")
		},
	})
}

func TestGetProductsWithFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cat1 := uuid.New()
	cat2 := uuid.New()

	fakeService := &fakeProductService{
		listProductsFn: func(ctx context.Context, params services.ListProductsParams) ([]*models.Product, int64, error) {
			return []*models.Product{
				{
					ID:    uuid.New(),
					Name:  "Test Product",
					Price: 12.5,
				},
			}, 1, nil
		},
	}

	controller := NewProductController(fakeService, newTestRedisClient())
	router := gin.New()
	router.GET("/products", controller.GetProducts)

	req := httptest.NewRequest(
		http.MethodGet,
		"/products?page=2&perPage=5&is_featured=true&categoryId="+cat2.String()+","+cat1.String()+"&minPrice=10.5&maxPrice=99.9&sort=price_asc",
		nil,
	)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if fakeService.listProductsCalled != 1 {
		t.Fatalf("expected list products to be called once, got %d", fakeService.listProductsCalled)
	}

	params := fakeService.lastParams
	if params.Page != 2 || params.PerPage != 5 {
		t.Fatalf("unexpected pagination params: page=%d perPage=%d", params.Page, params.PerPage)
	}

	if params.IsFeatured == nil || *params.IsFeatured != true {
		t.Fatalf("expected is_featured true, got %v", params.IsFeatured)
	}

	if params.Sort != "price_asc" {
		t.Fatalf("expected sort price_asc, got %q", params.Sort)
	}

	if params.MinPrice == nil || *params.MinPrice != 10.5 {
		t.Fatalf("expected minPrice 10.5, got %v", params.MinPrice)
	}

	if params.MaxPrice == nil || *params.MaxPrice != 99.9 {
		t.Fatalf("expected maxPrice 99.9, got %v", params.MaxPrice)
	}

	if len(params.CategoryID) != 2 {
		t.Fatalf("expected 2 category IDs, got %d", len(params.CategoryID))
	}

	categorySet := map[uuid.UUID]bool{
		params.CategoryID[0]: true,
		params.CategoryID[1]: true,
	}
	if !categorySet[cat1] || !categorySet[cat2] {
		t.Fatalf("expected category IDs to include %s and %s", cat1, cat2)
	}
}

func TestGetProductsInvalidCategoryID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductService{}
	controller := NewProductController(fakeService, newTestRedisClient())
	router := gin.New()
	router.GET("/products", controller.GetProducts)

	req := httptest.NewRequest(http.MethodGet, "/products?categoryId=not-a-uuid", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}

	if fakeService.listProductsCalled != 0 {
		t.Fatalf("expected list products not to be called, got %d", fakeService.listProductsCalled)
	}
}
