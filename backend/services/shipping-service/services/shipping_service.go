package services

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"shipping-service/models"
	"shipping-service/providers"
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
}

// NewShippingService creates a new ShippingService.
func NewShippingService(
	provider providers.ShippingProvider,
	logger *zap.Logger,
) ShippingService {
	return &shippingServiceImpl{
		provider: provider,
		logger:   logger,
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
		s.logger.Error("GetRates failed", zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to retrieve shipping rates: " + err.Error()}
	}

	if len(rates) == 0 {
		return nil, &ServiceError{StatusCode: 404, Message: "No shipping rates available for the given destination"}
	}

	return rates, nil
}
