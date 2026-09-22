package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"api-gateway/middlewares"
	"github.com/gin-gonic/gin"
)

func TestRequestIDMiddlewareNormalizesBothHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, headers := range map[string]http.Header{
		"request id wins":         {"X-Request-ID": []string{"req-1"}, "X-Correlation-ID": []string{"corr-1"}},
		"correlation id fallback": {"X-Correlation-ID": []string{"corr-2"}},
		"missing ids generate":    {},
	} {
		t.Run(name, func(t *testing.T) {
			r := gin.New()
			r.Use(middlewares.RequestIDMiddleware())
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for key, values := range headers {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Header().Get("X-Request-ID") == "" || w.Header().Get("X-Request-ID") != w.Header().Get("X-Correlation-ID") {
				t.Fatalf("headers were not normalized: request=%q correlation=%q", w.Header().Get("X-Request-ID"), w.Header().Get("X-Correlation-ID"))
			}
		})
	}
}
