package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"shipping-service/controllers"
	"shipping-service/models"
	"shipping-service/services"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// ---- concrete mock implementing services.ShippingService ----

type concreteMockSvc struct {
	rates    []models.ShippingRate
	rateErr  *services.ServiceError
	shipment *models.Shipment
	labelErr *services.ServiceError
	status   *models.TrackingStatus
	trackErr *services.ServiceError
}

func (m *concreteMockSvc) GetRates(ctx context.Context, req *models.ShippingRatesRequest) ([]models.ShippingRate, error) {
	if m.rateErr != nil {
		return m.rates, m.rateErr
	}
	return m.rates, nil
}
func (m *concreteMockSvc) CreateLabel(ctx context.Context, req *models.CreateLabelRequest) (*models.Shipment, error) {
	if m.labelErr != nil {
		return m.shipment, m.labelErr
	}
	return m.shipment, nil
}
func (m *concreteMockSvc) TrackShipment(ctx context.Context, code string) (*models.TrackingStatus, error) {
	if m.trackErr != nil {
		return m.status, m.trackErr
	}
	return m.status, nil
}

// ---- helpers ----

func setupRouter(svc services.ShippingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := controllers.NewShippingController(svc)

	r.POST("/shipping/rates", c.GetRates)
	r.POST("/shipping/labels", c.CreateLabel)
	r.GET("/shipping/track/:tracking_code", c.TrackShipment)
	return r
}

// ---- tests ----

func TestGetRates_Success(t *testing.T) {
	svc := &concreteMockSvc{
		rates: []models.ShippingRate{
			{Provider: "USPS", ServiceLevel: "Priority", Amount: 9.99, Currency: "USD", EstimatedDays: 2, RateID: "rate_abc"},
		},
	}
	r := setupRouter(svc)

	body := models.ShippingRatesRequest{
		WeightKg: 1.5,
		Destination: models.Address{
			Name: "Jane Doe", Street1: "456 Elm St",
			City: "New York", State: "NY", PostalCode: "10001", Country: "US",
		},
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/shipping/rates", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	rates, ok := resp["rates"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, rates, 1)
}

func TestGetRates_ServiceError(t *testing.T) {
	svc := &concreteMockSvc{
		rateErr: &services.ServiceError{StatusCode: 502, Message: "upstream error"},
	}
	r := setupRouter(svc)

	body := models.ShippingRatesRequest{WeightKg: 1.0}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/shipping/rates", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestGetRates_BadJSON(t *testing.T) {
	svc := &concreteMockSvc{}
	r := setupRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/shipping/rates", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateLabel_Success(t *testing.T) {
	svc := &concreteMockSvc{
		shipment: &models.Shipment{
			OrderID:      "order-1",
			TrackingCode: "1Z999AA10123456784",
			LabelURL:     "https://shippo.io/label.pdf",
			Status:       models.ShipmentStatusCreated,
		},
	}
	r := setupRouter(svc)

	body := models.CreateLabelRequest{
		OrderID: "order-1", UserID: "user-1", RateID: "rate_abc", WeightKg: 1.0,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/shipping/labels", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreateLabel_ProviderError(t *testing.T) {
	svc := &concreteMockSvc{
		labelErr: &services.ServiceError{StatusCode: 502, Message: "provider down"},
	}
	r := setupRouter(svc)

	body := models.CreateLabelRequest{OrderID: "order-2", UserID: "u2", RateID: "r2", WeightKg: 0.5}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/shipping/labels", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestTrackShipment_Success(t *testing.T) {
	svc := &concreteMockSvc{
		status: &models.TrackingStatus{TrackingCode: "TRK123", Status: "TRANSIT", Carrier: "USPS"},
	}
	r := setupRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/shipping/track/TRK123", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var status models.TrackingStatus
	_ = json.Unmarshal(w.Body.Bytes(), &status)
	assert.Equal(t, "TRANSIT", status.Status)
}

func TestTrackShipment_ServiceError(t *testing.T) {
	svc := &concreteMockSvc{
		trackErr: &services.ServiceError{StatusCode: 502, Message: "carrier unavailable"},
	}
	r := setupRouter(svc)

	req := httptest.NewRequest(http.MethodGet, "/shipping/track/TRK999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}
