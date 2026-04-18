package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestBindUpdateProductBody_AllowsImagesOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodPut, "/bff/admin/products/123", strings.NewReader(`{"images":["http://localhost:4566/shopswift/products/item.jpg"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	ctrl := &AdminProductController{logger: zap.NewNop()}

	body, ok := ctrl.bindUpdateProductBody(c)
	if !ok {
		t.Fatalf("expected images-only payload to be accepted, got status %d", w.Code)
	}

	var payload map[string][]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if got := payload["images"]; len(got) != 1 || got[0] != "http://localhost:4566/shopswift/products/item.jpg" {
		t.Fatalf("unexpected images payload: %#v", got)
	}
}

func TestGetPresignUpload_ForwardsQueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotPath string
	var gotQuery string
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload_url":"u","public_url":"p","key":"k","method":"PUT","expires_in":900}`))
	}))
	defer downstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/products/presign?sku=ABC-123&filename=front.jpg&content_type=image/jpeg", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	ctrl := &AdminProductController{
		logger:     zap.NewNop(),
		httpClient: downstream.Client(),
		baseURL:    downstream.URL,
	}

	ctrl.GetPresignUpload(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotPath != "/products/presign" {
		t.Fatalf("unexpected downstream path: %s", gotPath)
	}
	if gotQuery != "sku=ABC-123&filename=front.jpg&content_type=image/jpeg" {
		t.Fatalf("unexpected downstream query: %s", gotQuery)
	}
}

func TestPostProductImagePresign_ForwardsQueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotMethod string
	var gotPath string
	var gotQuery string
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload_url":"u","public_url":"p","key":"k","method":"PUT","expires_in":900}`))
	}))
	defer downstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/bff/admin/products/abc-123/images/presign?filename=front.jpg&content_type=image/jpeg", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "abc-123"}}

	ctrl := &AdminProductController{
		logger:     zap.NewNop(),
		httpClient: downstream.Client(),
		baseURL:    downstream.URL,
	}

	ctrl.PostProductImagePresign(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("unexpected downstream method: %s", gotMethod)
	}
	if gotPath != "/products/abc-123/images/presign" {
		t.Fatalf("unexpected downstream path: %s", gotPath)
	}
	if gotQuery != "filename=front.jpg&content_type=image/jpeg" {
		t.Fatalf("unexpected downstream query: %s", gotQuery)
	}
}
