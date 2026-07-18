package utils_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api-gateway/logger"
	"api-gateway/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestForwardRequest_StripsClientIdentityAndInjectsJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Log == nil {
		logger.Log = zap.NewNop()
	}

	var got http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	r := gin.New()
	r.GET("/inventory/:productId", func(c *gin.Context) {
		c.Set("user_id", "jwt-user")
		c.Set("email", "jwt@example.com")
		c.Set("role", "admin")
		utils.ForwardRequest(c, utils.ForwardOptions{TargetBase: upstream.URL + "/inventory"})
	})

	req := httptest.NewRequest(http.MethodGet, "/inventory/prod-1", nil)
	req.Header.Set("X-User-ID", "spoofed-user")
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-User-Email", "spoof@example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if got.Get("X-User-ID") != "jwt-user" {
		t.Fatalf("expected injected X-User-ID jwt-user, got %q", got.Get("X-User-ID"))
	}
	if got.Get("X-User-Role") != "admin" {
		t.Fatalf("expected injected X-User-Role admin, got %q", got.Get("X-User-Role"))
	}
	if got.Get("X-User-Email") != "jwt@example.com" {
		t.Fatalf("expected injected email, got %q", got.Get("X-User-Email"))
	}
	_, _ = io.Copy(io.Discard, strings.NewReader(""))
}
