package routes_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"order-service/controllers"
	"order-service/routes"
	"order-service/services"

	"github.com/gin-gonic/gin"
)

func TestRegisterOrderRoutes_IncludesRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := controllers.NewOrderController(&services.OrderService{})
	routes.RegisterOrderRoutes(r, ctrl)

	// Verify route is matched: with unauthenticated call, it should reach AuthMiddleware and return 401 (not 404)
	req, _ := http.NewRequest("POST", "/orders/11111111-1111-1111-1111-111111111111/refund", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected route /orders/:id/refund to be registered, got 404")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized from AuthMiddleware, got %d", w.Code)
	}
}
