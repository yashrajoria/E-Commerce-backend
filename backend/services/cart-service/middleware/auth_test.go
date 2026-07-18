package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cart-service/middleware"

	"github.com/gin-gonic/gin"
)

func TestRequireUser_RejectsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/cart", middleware.RequireUser(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRequireUser_AcceptsHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/cart", middleware.RequireUser(), func(c *gin.Context) {
		uid, _ := c.Get("user_id")
		if uid != "user-1" {
			t.Fatalf("expected user_id user-1, got %v", uid)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	req.Header.Set("X-User-ID", "user-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRequireUser_IgnoresCookieAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/cart", middleware.RequireUser(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	req.AddCookie(&http.Cookie{Name: "user_id", Value: "cookie-user"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when only cookie present, got %d", w.Code)
	}
}
