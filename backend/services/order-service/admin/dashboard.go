package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"order-service/models"
	"order-service/repository"

	"github.com/gin-gonic/gin"
	"github.com/yashrajoria/common/internalauth"
	"go.uber.org/zap"
)

// DashboardController serves the admin dashboard aggregation previously owned
// by bff-service. Order data (stats, counts, recent orders) is read in-process
// from the order repository; user and product totals/lists are fetched over
// the mesh from identity-service and catalog-service with the inbound
// gateway identity headers forwarded through.
type DashboardController struct {
	orderRepo    repositories.OrderRepository
	httpClient   *http.Client
	identityBase string
	catalogBase  string
	logger       *zap.Logger
}

func NewDashboardController(orderRepo repositories.OrderRepository, logger *zap.Logger) *DashboardController {
	return &DashboardController{
		orderRepo:    orderRepo,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		identityBase: strings.TrimRight(getEnvFallback("IDENTITY_SERVICE_URL", "http://identity-service:8081"), "/"),
		catalogBase:  strings.TrimRight(getEnvFallback("CATALOG_SERVICE_URL", "http://catalog-service:8082"), "/"),
		logger:       logger,
	}
}

func getEnvFallback(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return strings.TrimRight(value, "/")
	}
	return fallback
}

// --- downstream shapes ---

type countMetaResponse struct {
	Meta struct {
		Total int64 `json:"total"`
	} `json:"meta"`
}

type productsListResponse struct {
	Products []dashboardProduct `json:"products"`
	Meta     struct {
		Total int64 `json:"total"`
	} `json:"meta"`
}

type dashboardProduct struct {
	ID       string `json:"_id"`
	Name     string `json:"name"`
	Category struct {
		Name string `json:"name"`
	} `json:"category"`
	Price float64 `json:"price"`
}

// --- response shape (kept identical to the old BFF contract) ---

type dashboardResponse struct {
	KPIs struct {
		TotalRevenue struct {
			Value float64 `json:"value"`
			Trend float64 `json:"trend"`
		} `json:"totalRevenue"`
		TotalOrders struct {
			Value int     `json:"value"`
			Trend float64 `json:"trend"`
		} `json:"totalOrders"`
		TotalProducts struct {
			Value int     `json:"value"`
			Trend float64 `json:"trend"`
		} `json:"totalProducts"`
		ActiveUsers struct {
			Value int     `json:"value"`
			Trend float64 `json:"trend"`
		} `json:"activeUsers"`
	} `json:"kpis"`
	RevenueCharts struct {
		Monthly []struct {
			Name     string  `json:"name"`
			Revenue  float64 `json:"revenue"`
			Profit   float64 `json:"profit"`
			Expenses float64 `json:"expenses"`
		} `json:"monthly"`
		Weekly []struct {
			Name     string  `json:"name"`
			Revenue  float64 `json:"revenue"`
			Profit   float64 `json:"profit"`
			Expenses float64 `json:"expenses"`
		} `json:"weekly"`
	} `json:"revenueCharts"`
	TopProducts []struct {
		Name     string  `json:"name"`
		Category string  `json:"category"`
		Revenue  float64 `json:"revenue"`
		Units    int     `json:"units"`
		Trend    float64 `json:"trend"`
		Fill     int     `json:"fill"`
	} `json:"topProducts"`
	RecentActivity []struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Time        string `json:"time"`
		Variant     string `json:"variant"`
	} `json:"recentActivity"`
	CustomerInsights []struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	} `json:"customerInsights"`
}

// forwardHeaders carries the gateway-verified identity + mesh credential
// through to downstream admin endpoints.
func forwardHeaders(c *gin.Context) http.Header {
	h := http.Header{}
	for _, k := range []string{"X-User-ID", "X-User-Role", "X-User-Email", "X-Request-ID", "X-Correlation-ID"} {
		if v := c.GetHeader(k); v != "" {
			h.Set(k, v)
		}
	}
	return h
}

func (d *DashboardController) fetchJSON(ctx context.Context, url string, headers http.Header, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	internalauth.Apply(req)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (d *DashboardController) GetDashboardSummary(c *gin.Context) {
	ctx := c.Request.Context()
	headers := forwardHeaders(c)

	// 1. Order data — in-process (same DB, no HTTP)
	analytics, err := d.orderRepo.GetRevenueAnalytics(ctx)
	if err != nil {
		d.logger.Warn("dashboard: revenue analytics failed", zap.Error(err))
		analytics = map[string]interface{}{}
	}
	num := func(m map[string]interface{}, key string) float64 {
		switch v := m[key].(type) {
		case int64:
			return float64(v)
		case int:
			return float64(v)
		case float64:
			return v
		default:
			return 0
		}
	}

	var totalOrdersCount int64
	var allOrders []models.Order
	var orderErr error
	{
		// Reuse the repository directly: limit 1 for the count, 100 for recents.
		if _, total, err := d.orderRepo.FindAll(ctx, 1, 1); err == nil {
			totalOrdersCount = total
		} else {
			orderErr = err
		}
		orders, _, err := d.orderRepo.FindAll(ctx, 1, 100)
		if err == nil {
			allOrders = orders
		} else {
			orderErr = err
		}
	}

	// 2. Identity + catalog fan-out (bounded by client timeout)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var totalUsersCount, totalProductsCount int
	var allProducts []dashboardProduct
	var errs []error
	if orderErr != nil {
		errs = append(errs, fmt.Errorf("order data: %w", orderErr))
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		var resp countMetaResponse
		if err := d.fetchJSON(ctx, d.identityBase+"/users?page=1&page_size=1", headers, &resp); err != nil {
			mu.Lock()
			errs = append(errs, fmt.Errorf("identity users: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		totalUsersCount = int(resp.Meta.Total)
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		var countResp countMetaResponse
		if err := d.fetchJSON(ctx, d.catalogBase+"/products?page=1&limit=1", headers, &countResp); err != nil {
			mu.Lock()
			errs = append(errs, fmt.Errorf("catalog product count: %w", err))
			mu.Unlock()
			return
		}
		var listResp productsListResponse
		if err := d.fetchJSON(ctx, d.catalogBase+"/products?page=1&limit=100", headers, &listResp); err != nil {
			mu.Lock()
			errs = append(errs, fmt.Errorf("catalog products: %w", err))
			mu.Unlock()
			return
		}
		mu.Lock()
		totalProductsCount = int(countResp.Meta.Total)
		allProducts = listResp.Products
		mu.Unlock()
	}()
	wg.Wait()

	if len(errs) > 0 {
		d.logger.Warn("some dashboard sources failed", zap.Errors("errors", errs))
	}

	// 3. Compute statistics
	productMap := make(map[string]dashboardProduct, len(allProducts))
	for _, p := range allProducts {
		productMap[p.ID] = p
	}
	productSales := make(map[string]struct {
		revenue float64
		units   int
	})

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfYesterday := startOfToday.AddDate(0, 0, -1)

	var totalRevenue, revenueToday, revenueYesterday float64
	var totalOrdersToday, totalOrdersYesterday float64

	var recentActivities []struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Time        string `json:"time"`
		Variant     string `json:"variant"`
	}

	for i, order := range allOrders {
		if order.Status != "CANCELLED" && order.Status != "REFUNDED" && order.Status != "canceled" && order.Status != "refunded" {
			totalRevenue += float64(order.Amount) / 100
			if !order.CreatedAt.Before(startOfToday) {
				revenueToday += float64(order.Amount) / 100
				totalOrdersToday++
			} else if !order.CreatedAt.Before(startOfYesterday) {
				revenueYesterday += float64(order.Amount) / 100
				totalOrdersYesterday++
			}
		}

		for _, item := range order.OrderItems {
			stats := productSales[item.ProductID.String()]
			stats.revenue += float64(item.Price) * float64(item.Quantity) / 100
			stats.units += item.Quantity
			productSales[item.ProductID.String()] = stats
		}

		if i < 5 {
			desc := fmt.Sprintf("Order %s placed for $%.2f", order.ID.String()[:8], float64(order.Amount)/100)
			variant := "success"
			if order.Status == "pending_payment" || order.Status == "PENDING_PAYMENT" {
				variant = "warning"
			}
			recentActivities = append(recentActivities, struct {
				ID          string `json:"id"`
				Type        string `json:"type"`
				Description string `json:"description"`
				Time        string `json:"time"`
				Variant     string `json:"variant"`
			}{
				ID:          order.ID.String(),
				Type:        "Order " + order.Status,
				Description: desc,
				Time:        "Recently",
				Variant:     variant,
			})
		}
	}

	// 4. Construct response (contract identical to the old BFF endpoint)
	resp := dashboardResponse{}

	calcTrend := func(today, yesterday float64) float64 {
		if yesterday == 0 {
			if today > 0 {
				return 100.0
			}
			return 0.0
		}
		return ((today - yesterday) / yesterday) * 100.0
	}

	resp.KPIs.TotalRevenue.Value = num(analytics, "total_revenue")
	resp.KPIs.TotalRevenue.Trend = calcTrend(num(analytics, "revenue_today"), num(analytics, "revenue_yesterday"))
	resp.KPIs.TotalOrders.Value = int(totalOrdersCount)
	resp.KPIs.TotalOrders.Trend = calcTrend(num(analytics, "total_orders_today"), num(analytics, "total_orders_yesterday"))
	resp.KPIs.TotalProducts.Value = totalProductsCount
	resp.KPIs.TotalProducts.Trend = 0
	resp.KPIs.ActiveUsers.Value = totalUsersCount
	resp.KPIs.ActiveUsers.Trend = 0

	resp.RevenueCharts.Monthly = []struct {
		Name     string  `json:"name"`
		Revenue  float64 `json:"revenue"`
		Profit   float64 `json:"profit"`
		Expenses float64 `json:"expenses"`
	}{
		{Name: "Jan", Revenue: totalRevenue * 0.1, Profit: totalRevenue * 0.05, Expenses: totalRevenue * 0.05},
		{Name: "Feb", Revenue: totalRevenue * 0.15, Profit: totalRevenue * 0.08, Expenses: totalRevenue * 0.07},
		{Name: "Mar", Revenue: totalRevenue * 0.2, Profit: totalRevenue * 0.1, Expenses: totalRevenue * 0.1},
		{Name: "Apr", Revenue: totalRevenue * 0.25, Profit: totalRevenue * 0.12, Expenses: totalRevenue * 0.13},
		{Name: "May", Revenue: totalRevenue * 0.3, Profit: totalRevenue * 0.15, Expenses: totalRevenue * 0.15},
	}

	resp.RevenueCharts.Weekly = []struct {
		Name     string  `json:"name"`
		Revenue  float64 `json:"revenue"`
		Profit   float64 `json:"profit"`
		Expenses float64 `json:"expenses"`
	}{
		{Name: "Mon", Revenue: 6200, Profit: 3200, Expenses: 3000},
		{Name: "Tue", Revenue: 7800, Profit: 4100, Expenses: 3700},
		{Name: "Wed", Revenue: 8100, Profit: 4400, Expenses: 3700},
	}

	count := 0
	for pid, stats := range productSales {
		if count >= 5 {
			break
		}
		name := "Unknown Product"
		category := "General"
		if p, ok := productMap[pid]; ok {
			name = p.Name
			category = p.Category.Name
		}

		fill := 100 - (count * 20)
		if fill < 10 {
			fill = 10
		}

		resp.TopProducts = append(resp.TopProducts, struct {
			Name     string  `json:"name"`
			Category string  `json:"category"`
			Revenue  float64 `json:"revenue"`
			Units    int     `json:"units"`
			Trend    float64 `json:"trend"`
			Fill     int     `json:"fill"`
		}{
			Name:     name,
			Category: category,
			Revenue:  stats.revenue,
			Units:    stats.units,
			Trend:    15.0 - float64(count*2),
			Fill:     fill,
		})
		count++
	}

	if len(resp.TopProducts) == 0 {
		resp.TopProducts = append(resp.TopProducts, struct {
			Name     string  `json:"name"`
			Category string  `json:"category"`
			Revenue  float64 `json:"revenue"`
			Units    int     `json:"units"`
			Trend    float64 `json:"trend"`
			Fill     int     `json:"fill"`
		}{
			Name:     "No Sales Yet",
			Category: "-",
			Revenue:  0,
			Units:    0,
			Trend:    0,
			Fill:     0,
		})
	}

	resp.RecentActivity = recentActivities
	if len(resp.RecentActivity) == 0 {
		resp.RecentActivity = append(resp.RecentActivity, struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			Description string `json:"description"`
			Time        string `json:"time"`
			Variant     string `json:"variant"`
		}{
			ID:          "1",
			Type:        "System",
			Description: "Dashboard initialized.",
			Time:        "Just now",
			Variant:     "info",
		})
	}

	resp.CustomerInsights = []struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		{Name: "Returning", Value: int(float64(totalUsersCount) * 0.6)},
		{Name: "New", Value: int(float64(totalUsersCount) * 0.3)},
		{Name: "VIP", Value: int(float64(totalUsersCount) * 0.1)},
	}

	c.JSON(http.StatusOK, resp)
}
