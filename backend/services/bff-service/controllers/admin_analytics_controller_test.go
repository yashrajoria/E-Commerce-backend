package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"bff-service/utils"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newAnalyticsController(client *http.Client) *AdminAnalyticsController {
	return &AdminAnalyticsController{
		logger:           zap.NewNop(),
		httpClient:       client,
		orderServiceURL:  "http://downstream",
		userServiceURL:   "http://downstream",
		inventoryService: "http://downstream",
	}
}

func decodeAPIResponse(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &response))
	return response
}

func TestNewAdminAnalyticsController_UsesEnvAndDefaults(t *testing.T) {
	t.Setenv("ORDER_SERVICE_URL", "http://orders:9999/")
	t.Setenv("USER_SERVICE_URL", "")
	t.Setenv("INVENTORY_SERVICE_URL", "http://inventory:7777//")

	controller := NewAdminAnalyticsController(zap.NewNop(), &http.Client{})

	assert.Equal(t, "http://orders:9999", controller.orderServiceURL)
	assert.Equal(t, "http://user-service:8085", controller.userServiceURL)
	assert.Equal(t, "http://inventory:7777", controller.inventoryService)
}

func TestAdminAnalyticsController_EndpointsForwardToExpectedPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		handler      func(*AdminAnalyticsController, *gin.Context)
		setURL       func(*AdminAnalyticsController, string)
		expectedPath string
	}{
		{
			name:    "sales_report",
			handler: (*AdminAnalyticsController).SalesReport,
			setURL: func(c *AdminAnalyticsController, base string) {
				c.orderServiceURL = base
			},
			expectedPath: "/orders/admin/stats",
		},
		{
			name:    "users_report",
			handler: (*AdminAnalyticsController).UsersReport,
			setURL: func(c *AdminAnalyticsController, base string) {
				c.userServiceURL = base
			},
			expectedPath: "/users",
		},
		{
			name:    "inventory_report",
			handler: (*AdminAnalyticsController).InventoryReport,
			setURL: func(c *AdminAnalyticsController, base string) {
				c.inventoryService = base
			},
			expectedPath: "/inventory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				assert.Equal(t, "admin-1", r.Header.Get("X-User-ID"))
				assert.Equal(t, "admin", r.Header.Get("X-User-Role"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			defer downstream.Close()

			controller := newAnalyticsController(downstream.Client())
			tt.setURL(controller, downstream.URL)

			router := gin.New()
			router.GET("/report", func(c *gin.Context) {
				tt.handler(controller, c)
			})

			req := httptest.NewRequest(http.MethodGet, "/report", nil)
			req.Header.Set("X-User-ID", "admin-1")
			req.Header.Set("X-User-Role", "admin")
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			require.Equal(t, http.StatusOK, resp.Code)
			assert.Equal(t, tt.expectedPath, gotPath)

			body := decodeAPIResponse(t, resp.Body.Bytes())
			assert.Equal(t, true, body["success"])
			data, ok := body["data"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, true, data["ok"])
		})
	}
}

func TestAdminAnalyticsController_ForwardReportStatusHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		downstreamCode  int
		downstreamBody  string
		expectedCode    int
		expectedSuccess bool
		expectedError   string
	}{
		{
			name:            "bad_request_maps_error",
			downstreamCode:  http.StatusBadRequest,
			downstreamBody:  `{"error":"bad"}`,
			expectedCode:    http.StatusBadRequest,
			expectedSuccess: false,
			expectedError:   "invalid request",
		},
		{
			name:            "not_found_is_passthrough",
			downstreamCode:  http.StatusNotFound,
			downstreamBody:  `{"error":"missing"}`,
			expectedCode:    http.StatusNotFound,
			expectedSuccess: false,
			expectedError:   "not found",
		},
		{
			name:            "server_error_becomes_unavailable",
			downstreamCode:  http.StatusInternalServerError,
			downstreamBody:  `{"error":"boom"}`,
			expectedCode:    http.StatusServiceUnavailable,
			expectedSuccess: false,
			expectedError:   "downstream service unavailable",
		},
		{
			name:            "success_with_invalid_json_returns_raw",
			downstreamCode:  http.StatusOK,
			downstreamBody:  `not-json`,
			expectedCode:    http.StatusOK,
			expectedSuccess: true,
			expectedError:   "",
		},
		{
			name:            "success_with_empty_body_returns_empty_object",
			downstreamCode:  http.StatusOK,
			downstreamBody:  ``,
			expectedCode:    http.StatusOK,
			expectedSuccess: true,
			expectedError:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.downstreamCode)
				_, _ = io.WriteString(w, tt.downstreamBody)
			}))
			defer downstream.Close()

			controller := newAnalyticsController(downstream.Client())
			controller.orderServiceURL = downstream.URL

			router := gin.New()
			router.GET("/report", controller.SalesReport)

			req := httptest.NewRequest(http.MethodGet, "/report", nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)

			require.Equal(t, tt.expectedCode, resp.Code)
			body := decodeAPIResponse(t, resp.Body.Bytes())
			assert.Equal(t, tt.expectedSuccess, body["success"])
			assert.Equal(t, tt.expectedError, body["error"])

			if tt.name == "success_with_invalid_json_returns_raw" {
				data, ok := body["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, "not-json", data["raw"])
			}
			if tt.name == "success_with_empty_body_returns_empty_object" {
				data, ok := body["data"].(map[string]interface{})
				require.True(t, ok)
				assert.Len(t, data, 0)
			}
		})
	}
}

func TestAdminAnalyticsController_ForwardReportNetworkError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	controller := newAnalyticsController(client)

	router := gin.New()
	router.GET("/report", controller.SalesReport)

	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusServiceUnavailable, resp.Code)
	body := decodeAPIResponse(t, resp.Body.Bytes())
	assert.Equal(t, false, body["success"])
	assert.Equal(t, "downstream service unavailable", body["error"])
}

func TestAdminAnalyticsController_ConcurrentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"items":[1,2,3]}`)
	}))
	defer downstream.Close()

	controller := newAnalyticsController(downstream.Client())
	controller.inventoryService = downstream.URL

	router := gin.New()
	router.GET("/report", controller.InventoryReport)

	const workers = 40
	var wg sync.WaitGroup
	wg.Add(workers)

	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/report", nil)
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				errCh <- errors.New("non-200 response")
				return
			}
			var parsed utils.APIResponse
			if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
				errCh <- err
				return
			}
			if !parsed.Success {
				errCh <- errors.New("unexpected unsuccessful response")
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
}

func FuzzDecodeResponseBody(f *testing.F) {
	seeds := [][]byte{
		[]byte(""),
		[]byte("{}"),
		[]byte("{\"a\":1}"),
		[]byte("not-json"),
		[]byte("[]"),
	}
	for _, seed := range seeds {
		f.Add(string(seed))
	}

	f.Fuzz(func(t *testing.T, body string) {
		decoded := decodeResponseBody([]byte(body))
		require.NotNil(t, decoded)

		if strings.TrimSpace(body) == "" {
			m, ok := decoded.(gin.H)
			require.True(t, ok)
			require.Len(t, m, 0)
		}
	})
}
