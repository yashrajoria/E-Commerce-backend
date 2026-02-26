package services

import (
	"context"
	"encoding/json"
	"fmt"
	"shipping-service/models"
	"shipping-service/providers"
	"shipping-service/repository"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
	CreateLabel(ctx context.Context, req *models.CreateLabelRequest) (*models.Shipment, *ServiceError)
	TrackShipment(ctx context.Context, trackingCode string) (*models.TrackingStatus, *ServiceError)
}

type shippingServiceImpl struct {
	repo        repository.ShipmentRepository
	provider    providers.ShippingProvider
	snsClient   aws_pkg.SNSPublisher
	snsTopicArn string
	originAddr  models.Address // default ship-from address (warehouse)
	logger      *zap.Logger
}

// NewShippingService creates a new ShippingService.
func NewShippingService(
	repo repository.ShipmentRepository,
	provider providers.ShippingProvider,
	snsClient aws_pkg.SNSPublisher,
	snsTopicArn string,
	originAddr models.Address,
	logger *zap.Logger,
) ShippingService {
	return &shippingServiceImpl{
		repo:        repo,
		provider:    provider,
		snsClient:   snsClient,
		snsTopicArn: snsTopicArn,
		originAddr:  originAddr,
		logger:      logger,
	}
}

// GetRates queries the shipping provider for available rates.
func (s *shippingServiceImpl) GetRates(ctx context.Context, req *models.ShippingRatesRequest) ([]models.ShippingRate, *ServiceError) {
	rates, err := s.provider.GetRates(req.WeightKg, s.originAddr, req.Destination)
	if err != nil {
		s.logger.Error("GetRates failed", zap.Error(err))
		return nil, &ServiceError{StatusCode: 502, Message: "Failed to retrieve shipping rates: " + err.Error()}
	}

	if len(rates) == 0 {
		return nil, &ServiceError{StatusCode: 404, Message: "No shipping rates available for the given destination"}
	}

	return rates, nil
}

// CreateLabel creates a shipping label and persists the shipment record.
func (s *shippingServiceImpl) CreateLabel(ctx context.Context, req *models.CreateLabelRequest) (*models.Shipment, *ServiceError) {
	// Check for duplicate
	if existing, err := s.repo.FindByOrderID(ctx, req.OrderID); err == nil && existing != nil {
		return existing, nil
	}

	info, err := s.provider.CreateLabel(*req)
	if err != nil {
		s.logger.Error("CreateLabel failed", zap.Error(err))
		return nil, &ServiceError{StatusCode: 502, Message: "Failed to create shipping label: " + err.Error()}
	}

	originBytes, _ := json.Marshal(req.Origin)
	destBytes, _ := json.Marshal(req.Destination)

	shipment := &models.Shipment{
		OrderID:         req.OrderID,
		UserID:          req.UserID,
		TrackingCode:    info.TrackingCode,
		LabelURL:        info.LabelURL,
		TrackingURL:     info.TrackingURL,
		ShippoObjectID:  info.ShippoObjectID,
		Status:          models.ShipmentStatusCreated,
		WeightKg:        req.WeightKg,
		OriginJSON:      string(originBytes),
		DestinationJSON: string(destBytes),
	}

	if err := s.repo.Create(ctx, shipment); err != nil {
		s.logger.Error("Failed to persist shipment", zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to save shipment record"}
	}

	s.logger.Info("Shipment created",
		zap.String("order_id", req.OrderID),
		zap.String("tracking_code", info.TrackingCode),
	)

	// Publish shipment_created SNS event
	s.publishEvent(ctx, models.ShipmentCreatedEvent{
		EventType:    "shipment_created",
		ShipmentID:   shipment.ID.String(),
		OrderID:      shipment.OrderID,
		UserID:       shipment.UserID,
		TrackingCode: shipment.TrackingCode,
		Carrier:      shipment.Carrier,
		LabelURL:     shipment.LabelURL,
		Timestamp:    time.Now(),
	})

	return shipment, nil
}

// TrackShipment fetches tracking status from the provider and updates the DB record.
func (s *shippingServiceImpl) TrackShipment(ctx context.Context, trackingCode string) (*models.TrackingStatus, *ServiceError) {
	// Fetch current DB record to get carrier
	dbRecord, err := s.repo.FindByTrackingCode(ctx, trackingCode)
	if err != nil && err != gorm.ErrRecordNotFound {
		s.logger.Warn("Shipment DB record not found for tracking code", zap.String("code", trackingCode))
	}

	carrier := ""
	if dbRecord != nil {
		carrier = dbRecord.Carrier
	}

	status, err := s.provider.TrackShipment(carrier, trackingCode)
	if err != nil {
		s.logger.Error("TrackShipment failed", zap.Error(err))
		return nil, &ServiceError{StatusCode: 502, Message: "Failed to fetch tracking status: " + err.Error()}
	}

	// Update DB if record exists and status changed
	if dbRecord != nil && dbRecord.Status != status.Status {
		dbRecord.Status = status.Status
		if updateErr := s.repo.Update(ctx, dbRecord); updateErr != nil {
			s.logger.Warn("Failed to update shipment status", zap.Error(updateErr))
		}

		s.publishEvent(ctx, models.ShipmentUpdatedEvent{
			EventType:    "shipment_updated",
			ShipmentID:   dbRecord.ID.String(),
			OrderID:      dbRecord.OrderID,
			TrackingCode: trackingCode,
			Status:       status.Status,
			Timestamp:    time.Now(),
		})
	}

	return &status, nil
}

// publishEvent marshals an event and publishes it to SNS (non-fatal on error).
func (s *shippingServiceImpl) publishEvent(ctx context.Context, event interface{}) {
	if s.snsClient == nil || s.snsTopicArn == "" {
		s.logger.Warn("SNS not configured, skipping event publish")
		return
	}
	b, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("Failed to marshal SNS event", zap.Error(err))
		return
	}
	if err := s.snsClient.Publish(ctx, s.snsTopicArn, b); err != nil {
		s.logger.Error("Failed to publish SNS event", zap.Error(err))
		return
	}
	s.logger.Info("Published SNS event", zap.String("topic", s.snsTopicArn))
}

// FormatProviderError formats a provider error message.
func FormatProviderError(msg string) string {
	return fmt.Sprintf("shipping provider error: %s", msg)
}
