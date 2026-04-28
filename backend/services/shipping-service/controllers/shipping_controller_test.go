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

type concreteMockSvc struct {
	rates   []models.ShippingRate
	rateErr *services.ServiceError
}

func (m *concreteMockSvc) GetRates(ctx context.Context, req *models.ShippingRatesRequest) ([]models.ShippingRate, *services.ServiceError) {
	if m.rateErr != nil {
		return m.rates, m.rateErr
	}
	return m.rates, nil
}

func setupRouter(svc services.ShippingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := controllers.NewShippingController(svc)

	r.POST("/shipping/rates", c.GetRates)
	return r
}

func TestGetRates_Success(t *testing.T) {
	svc := &concreteMockSvc{
		rates: []models.ShippingRate{
			{Provider: "Internal", ServiceLevel: "Standard", Amount: 9.99, Currency: "USD", EstimatedDays: 2, RateID: "us-standard-0-1"},
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
		rateErr: &services.ServiceError{StatusCode: 500, Message: "rate configuration error"},
	}
	r := setupRouter(svc)

	body := models.ShippingRatesRequest{WeightKg: 1.0}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/shipping/rates", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
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
