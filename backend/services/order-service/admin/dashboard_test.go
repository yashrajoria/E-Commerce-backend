package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"order-service/models"
	repositories "order-service/repository"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type seriesRepo struct {
	repositories.OrderRepository
}

func (seriesRepo) GetRevenueAnalytics(context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (seriesRepo) FindAll(context.Context, int, int) ([]models.Order, int64, error) {
	return nil, 0, nil
}
func (seriesRepo) RevenueSeries(_ context.Context, unit string, _ time.Time) ([]repositories.RevenuePoint, error) {
	now := time.Now().UTC()
	if unit == "month" {
		return []repositories.RevenuePoint{{Bucket: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), Revenue: 12345}}, nil
	}
	return []repositories.RevenuePoint{{Bucket: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), Revenue: 500}}, nil
}

func TestDashboard_ChartsUseRealSeries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	d := &DashboardController{orderRepo: seriesRepo{}, httpClient: &http.Client{Timeout: 50 * time.Millisecond},
		identityBase: "http://127.0.0.1:1", catalogBase: "http://127.0.0.1:1", logger: zap.NewNop()}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	d.GetDashboardSummary(c)

	var resp struct {
		RevenueCharts struct {
			Monthly []struct{ Revenue float64 } `json:"monthly"`
			Weekly  []struct{ Revenue float64 } `json:"weekly"`
		} `json:"revenueCharts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	m, wk := resp.RevenueCharts.Monthly, resp.RevenueCharts.Weekly
	if len(m) != 6 || len(wk) != 7 {
		t.Fatalf("want 6 months / 7 days, got %d / %d", len(m), len(wk))
	}
	if m[5].Revenue != 123.45 || wk[6].Revenue != 5 {
		t.Fatalf("current month/day revenue wrong: %v %v", m[5].Revenue, wk[6].Revenue)
	}
	if m[0].Revenue != 0 || wk[0].Revenue != 0 {
		t.Fatal("empty buckets must be zero, not invented")
	}
}
