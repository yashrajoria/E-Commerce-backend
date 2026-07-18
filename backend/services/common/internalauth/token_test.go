package internalauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_SERVICE_TOKEN", "test-token-value")

	r := gin.New()
	r.GET("/internal/x", Require(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	t.Run("missing token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d want 401", w.Code)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
		req.Header.Set(Header, "nope")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("got %d want 401", w.Code)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
		req.Header.Set(Header, "test-token-value")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d want 200", w.Code)
		}
	})
}

func TestRequireUnconfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_SERVICE_TOKEN", "")

	r := gin.New()
	r.GET("/internal/x", Require(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/x", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d want 503", w.Code)
	}
}

func TestApply(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_TOKEN", "mesh-secret")
	req := httptest.NewRequest(http.MethodGet, "http://example/", nil)
	Apply(req)
	if got := req.Header.Get(Header); got != "mesh-secret" {
		t.Fatalf("header=%q", got)
	}
}
