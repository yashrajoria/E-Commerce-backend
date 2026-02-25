package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"promotion-service/controllers"
	"promotion-service/models"
	"promotion-service/services"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- Mock CouponService ---

type mockCouponService struct {
	createFn   func(ctx context.Context, req *models.CreateCouponRequest) (*models.Coupon, *services.ServiceError)
	validateFn func(ctx context.Context, req *models.ValidateCouponRequest) (*models.ValidateCouponResponse, *services.ServiceError)
	getFn      func(ctx context.Context, code string) (*models.Coupon, *services.ServiceError)
	deactFn    func(ctx context.Context, code string) *services.ServiceError
	listFn     func(ctx context.Context, page, limit int) ([]models.Coupon, int64, *services.ServiceError)
}

func (m *mockCouponService) CreateCoupon(ctx context.Context, req *models.CreateCouponRequest) (*models.Coupon, *services.ServiceError) {
	return m.createFn(ctx, req)
}
func (m *mockCouponService) ValidateCoupon(ctx context.Context, req *models.ValidateCouponRequest) (*models.ValidateCouponResponse, *services.ServiceError) {
	return m.validateFn(ctx, req)
}
func (m *mockCouponService) GetCoupon(ctx context.Context, code string) (*models.Coupon, *services.ServiceError) {
	return m.getFn(ctx, code)
}
func (m *mockCouponService) DeactivateCoupon(ctx context.Context, code string) *services.ServiceError {
	return m.deactFn(ctx, code)
}
func (m *mockCouponService) ListCoupons(ctx context.Context, page, limit int) ([]models.Coupon, int64, *services.ServiceError) {
	return m.listFn(ctx, page, limit)
}

// --- Helpers ---

func adminCtx(r *gin.Engine) *gin.Engine {
	r.Use(func(c *gin.Context) {
		c.Set("userID", "user-admin-id")
		c.Set("role", "admin")
		c.Next()
	})
	return r
}

func setupRouter(svc services.CouponService) *gin.Engine {
	r := gin.New()
	cc := controllers.NewCouponController(svc)

	r.Use(func(c *gin.Context) {
		c.Set("userID", "user-test-id")
		c.Set("role", "admin")
		c.Next()
	})

	r.POST("/coupons", cc.CreateCoupon)
	r.POST("/coupons/validate", cc.ValidateCoupon)
	r.GET("/coupons/:code", cc.GetCoupon)
	r.DELETE("/coupons/:code", cc.DeactivateCoupon)
	r.GET("/coupons", cc.ListCoupons)
	return r
}

// --- Tests ---

func TestController_CreateCoupon_Success(t *testing.T) {
	svc := &mockCouponService{
		createFn: func(_ context.Context, req *models.CreateCouponRequest) (*models.Coupon, *services.ServiceError) {
			return &models.Coupon{
				ID:        uuid.New(),
				Code:      req.Code,
				Type:      req.Type,
				Value:     req.Value,
				ExpiresAt: req.ExpiresAt,
				Active:    true,
				CreatedAt: time.Now(),
			}, nil
		},
	}
	r := setupRouter(svc)

	payload := models.CreateCouponRequest{
		Code:       "NEW10",
		Type:       models.CouponTypePercentage,
		Value:      10,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		UsageLimit: 50,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, "/coupons", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotNil(t, resp["coupon"])
}

func TestController_CreateCoupon_BadRequest(t *testing.T) {
	svc := &mockCouponService{}
	r := setupRouter(svc)

	req, _ := http.NewRequest(http.MethodPost, "/coupons", bytes.NewBufferString(`{"code":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestController_ValidateCoupon_Valid(t *testing.T) {
	svc := &mockCouponService{
		validateFn: func(_ context.Context, req *models.ValidateCouponRequest) (*models.ValidateCouponResponse, *services.ServiceError) {
			return &models.ValidateCouponResponse{
				Valid:          true,
				Code:           req.Code,
				Type:           models.CouponTypePercentage,
				DiscountAmount: 10.0,
				Message:        "Coupon applied successfully",
			}, nil
		},
	}
	r := setupRouter(svc)

	body, _ := json.Marshal(map[string]interface{}{
		"code":       "SAVE10",
		"cart_total": 100.0,
	})
	req, _ := http.NewRequest(http.MethodPost, "/coupons/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp models.ValidateCouponResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp.Valid)
	assert.Equal(t, 10.0, resp.DiscountAmount)
}

func TestController_GetCoupon_NotFound(t *testing.T) {
	svc := &mockCouponService{
		getFn: func(_ context.Context, _ string) (*models.Coupon, *services.ServiceError) {
			return nil, &services.ServiceError{StatusCode: 404, Message: "Coupon not found"}
		},
	}
	r := setupRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/coupons/GHOST", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestController_GetCoupon_Success(t *testing.T) {
	svc := &mockCouponService{
		getFn: func(_ context.Context, code string) (*models.Coupon, *services.ServiceError) {
			return &models.Coupon{
				ID:   uuid.New(),
				Code: code,
				Type: models.CouponTypeFlat,
				Value: 20,
				Active: true,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
	}
	r := setupRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/coupons/FLAT20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestController_DeactivateCoupon_Success(t *testing.T) {
	svc := &mockCouponService{
		deactFn: func(_ context.Context, _ string) *services.ServiceError {
			return nil
		},
	}
	r := setupRouter(svc)

	req, _ := http.NewRequest(http.MethodDelete, "/coupons/SAVE10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestController_DeactivateCoupon_NotFound(t *testing.T) {
	svc := &mockCouponService{
		deactFn: func(_ context.Context, _ string) *services.ServiceError {
			return &services.ServiceError{StatusCode: 404, Message: "Coupon not found"}
		},
	}
	r := setupRouter(svc)

	req, _ := http.NewRequest(http.MethodDelete, "/coupons/GHOST", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestController_ListCoupons(t *testing.T) {
	svc := &mockCouponService{
		listFn: func(_ context.Context, _, _ int) ([]models.Coupon, int64, *services.ServiceError) {
			return []models.Coupon{
				{ID: uuid.New(), Code: "A", Type: models.CouponTypeFlat, Value: 5, Active: true, ExpiresAt: time.Now().Add(time.Hour)},
				{ID: uuid.New(), Code: "B", Type: models.CouponTypePercentage, Value: 10, Active: true, ExpiresAt: time.Now().Add(time.Hour)},
			}, 2, nil
		},
	}
	r := setupRouter(svc)

	req, _ := http.NewRequest(http.MethodGet, "/coupons?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotNil(t, resp["coupons"])
}
