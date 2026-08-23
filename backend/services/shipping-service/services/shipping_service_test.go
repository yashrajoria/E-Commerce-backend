package services_test

import (
	"context"
	"errors"
	"fmt"
	"shipping-service/models"
	"shipping-service/providers"
	"shipping-service/services"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type typedMockProvider struct {
	rates    []models.ShippingRate
	ratesErr error
}

func (p *typedMockProvider) GetRates(weightKg float64, dest models.Address) ([]models.ShippingRate, error) {
	return p.rates, p.ratesErr
}

func newTestService(provider *typedMockProvider) services.ShippingService {
	logger, _ := zap.NewDevelopment()
	return services.NewShippingService(provider, logger)
}

func TestGetRates_Success(t *testing.T) {
	provider := &typedMockProvider{
		rates: []models.ShippingRate{{Provider: "Internal", Amount: 8.50, Currency: "USD"}},
	}
	svc := newTestService(provider)

	req := &models.ShippingRatesRequest{WeightKg: 1.0, Destination: models.Address{Country: "US"}}
	rates, err := svc.GetRates(context.Background(), req)

	assert.Nil(t, err)
	assert.Len(t, rates, 1)
	assert.Equal(t, "Internal", rates[0].Provider)
}

func TestGetRates_ProviderError(t *testing.T) {
	provider := &typedMockProvider{ratesErr: errors.New("invalid rate configuration")}
	svc := newTestService(provider)

	_, svcErr := svc.GetRates(context.Background(), &models.ShippingRatesRequest{WeightKg: 1.0})
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 500, se.StatusCode)
	}
}

func TestGetRates_UnserviceableDestination(t *testing.T) {
	provider := &typedMockProvider{ratesErr: fmt.Errorf("%w: invalid or missing country code \"\"", providers.ErrUnserviceableDestination)}
	svc := newTestService(provider)

	_, svcErr := svc.GetRates(context.Background(), &models.ShippingRatesRequest{WeightKg: 1.0, Destination: models.Address{Country: ""}})
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 422, se.StatusCode)
	}
}

func TestGetRates_EmptyRates(t *testing.T) {
	provider := &typedMockProvider{rates: []models.ShippingRate{}}
	svc := newTestService(provider)

	_, svcErr := svc.GetRates(context.Background(), &models.ShippingRatesRequest{WeightKg: 0.5})
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 404, se.StatusCode)
	}
}
