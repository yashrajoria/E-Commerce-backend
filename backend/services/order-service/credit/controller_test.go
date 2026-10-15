package credit_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"order-service/credit"
	"order-service/routes"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *credit.Service, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	svc := credit.NewService(db)
	ctrl := credit.NewController(svc)

	meshToken := "test-internal-token-xyz"
	t.Setenv("INTERNAL_SERVICE_TOKEN", meshToken)

	r := gin.New()
	routes.RegisterOrderRoutes(r, nil, ctrl)
	return r, svc, meshToken
}

func TestStoreCreditRoutes_MeshAuthRequired(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	storeID := uuid.New().String()
	userID := uuid.New().String()

	// 1. Without mesh token -> 401 Unauthorized
	req, _ := http.NewRequest(http.MethodGet, "/store-credit/balance", nil)
	req.Header.Set("X-Store-ID", storeID)
	req.Header.Set("X-User-ID", userID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestStoreCreditRoutes_BalanceAndLedger(t *testing.T) {
	r, svc, meshToken := setupTestRouter(t)

	storeID := uuid.New()
	userID := uuid.New()
	actor := uuid.New()

	// Seed credit
	require.NoError(t, svc.Grant(t.Context(), storeID, userID, 1250, "GBP", "promo credit", "seed-idem", &actor, nil))

	// Query balance with valid headers
	req, _ := http.NewRequest(http.MethodGet, "/store-credit/balance", nil)
	req.Header.Set("X-Internal-Service-Token", meshToken)
	req.Header.Set("X-Store-ID", storeID.String())
	req.Header.Set("X-User-ID", userID.String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, float64(1250), body["available_amount"])
	assert.Equal(t, float64(0), body["held_amount"])
	assert.Equal(t, float64(1250), body["total_amount"])

	// Query ledger
	reqLedger, _ := http.NewRequest(http.MethodGet, "/store-credit/ledger", nil)
	reqLedger.Header.Set("X-Internal-Service-Token", meshToken)
	reqLedger.Header.Set("X-Store-ID", storeID.String())
	reqLedger.Header.Set("X-User-ID", userID.String())
	wLedger := httptest.NewRecorder()
	r.ServeHTTP(wLedger, reqLedger)

	assert.Equal(t, http.StatusOK, wLedger.Code)
	var ledgerBody map[string]interface{}
	require.NoError(t, json.Unmarshal(wLedger.Body.Bytes(), &ledgerBody))
	entries := ledgerBody["entries"].([]interface{})
	assert.Len(t, entries, 1)
}

func TestStoreCreditRoutes_Grant_RBAC(t *testing.T) {
	r, svc, meshToken := setupTestRouter(t)

	storeID := uuid.New()
	adminUser := uuid.New()
	customerUser := uuid.New()
	targetCustomer := uuid.New()

	payload, _ := json.Marshal(map[string]interface{}{
		"user_id":  targetCustomer.String(),
		"amount":   500,
		"currency": "GBP",
		"reason":   "appeasement credit",
	})

	// 1. Regular customer attempting to grant credit -> 403 Forbidden
	reqCustomer, _ := http.NewRequest(http.MethodPost, "/store-credit/grants", bytes.NewReader(payload))
	reqCustomer.Header.Set("X-Internal-Service-Token", meshToken)
	reqCustomer.Header.Set("X-Store-ID", storeID.String())
	reqCustomer.Header.Set("X-User-ID", customerUser.String())
	reqCustomer.Header.Set("X-User-Role", "customer")
	reqCustomer.Header.Set("Idempotency-Key", "customer-grant-attempt")
	reqCustomer.Header.Set("Content-Type", "application/json")
	wCustomer := httptest.NewRecorder()
	r.ServeHTTP(wCustomer, reqCustomer)
	assert.Equal(t, http.StatusForbidden, wCustomer.Code)

	// 2. Global Admin granting credit -> 201 Created
	reqAdmin, _ := http.NewRequest(http.MethodPost, "/store-credit/grants", bytes.NewReader(payload))
	reqAdmin.Header.Set("X-Internal-Service-Token", meshToken)
	reqAdmin.Header.Set("X-Store-ID", storeID.String())
	reqAdmin.Header.Set("X-User-ID", adminUser.String())
	reqAdmin.Header.Set("X-User-Role", "admin")
	reqAdmin.Header.Set("Idempotency-Key", "admin-grant-valid")
	reqAdmin.Header.Set("Content-Type", "application/json")
	wAdmin := httptest.NewRecorder()
	r.ServeHTTP(wAdmin, reqAdmin)
	assert.Equal(t, http.StatusCreated, wAdmin.Code)

	// Verify balance of target customer
	avail, _, _, _ := svc.Balance(t.Context(), storeID, targetCustomer)
	assert.Equal(t, int64(500), avail)
}

func TestStoreCreditRoutes_AdjustmentDebit_RBAC(t *testing.T) {
	r, svc, meshToken := setupTestRouter(t)

	storeID := uuid.New()
	adminUser := uuid.New()
	targetCustomer := uuid.New()

	// Initial grant
	require.NoError(t, svc.Grant(t.Context(), storeID, targetCustomer, 1000, "GBP", "seed", "seed-adj", nil, nil))

	// Debit 300 via adjustment endpoint
	payload, _ := json.Marshal(map[string]interface{}{
		"user_id":  targetCustomer.String(),
		"amount":   -300,
		"currency": "GBP",
		"reason":   "fraud correction",
	})

	req, _ := http.NewRequest(http.MethodPost, "/store-credit/adjustments", bytes.NewReader(payload))
	req.Header.Set("X-Internal-Service-Token", meshToken)
	req.Header.Set("X-Store-ID", storeID.String())
	req.Header.Set("X-User-ID", adminUser.String())
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("Idempotency-Key", "adj-debit-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	avail, _, _, _ := svc.Balance(t.Context(), storeID, targetCustomer)
	assert.Equal(t, int64(700), avail)
}
