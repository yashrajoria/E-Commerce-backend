package services

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
	"order-service/shipping/models"
	"order-service/shipping/providers"
)

// ServiceError is a typed error with an HTTP status code.
type ServiceError struct {
	StatusCode int
	Message    string
}

func (e *ServiceError) Error() string { return e.Message }

// ShippingService defines the business logic interface.
type ShippingService interface {
	GetRates(ctx context.Context, req *models.ShippingRatesRequest) ([]models.ShippingRate, *ServiceError)
}

type shippingServiceImpl struct {
	provider providers.ShippingProvider
	logger   *zap.Logger
	currency string
}

// NewShippingService creates a new ShippingService.
// currency overrides the provider default (e.g. STORE_CURRENCY); empty keeps USD.
func NewShippingService(
	provider providers.ShippingProvider,
	logger *zap.Logger,
	currency ...string,
) ShippingService {
	cur := "USD"
	if len(currency) > 0 && currency[0] != "" {
		cur = strings.ToUpper(strings.TrimSpace(currency[0]))
	}
	return &shippingServiceImpl{
		provider: provider,
		logger:   logger,
		currency: cur,
	}
}

// GetRates queries the shipping provider for available rates.
func (s *shippingServiceImpl) GetRates(ctx context.Context, req *models.ShippingRatesRequest) ([]models.ShippingRate, *ServiceError) {
	rates, err := s.provider.GetRates(req.WeightKg, req.Destination)
	if err != nil {
		if errors.Is(err, providers.ErrUnserviceableDestination) {
			s.logger.Warn("GetRates: unserviceable destination", zap.Error(err))
			return nil, &ServiceError{StatusCode: 422, Message: err.Error()}
		}
		if errors.Is(err, providers.ErrInvalidWeight) {
			s.logger.Warn("GetRates: invalid weight", zap.Error(err))
			return nil, &ServiceError{StatusCode: 400, Message: err.Error()}
		}
		s.logger.Error("GetRates failed", zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to retrieve shipping rates: " + err.Error()}
	}

	if len(rates) == 0 {
		return nil, &ServiceError{StatusCode: 404, Message: "No shipping rates available for the given destination"}
	}

	for i := range rates {
		if s.currency != "" {
			rates[i].Currency = s.currency
		}
	}

	return rates, nil
}
