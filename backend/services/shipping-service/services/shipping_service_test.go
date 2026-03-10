package services_test

import (
	"context"
	"errors"
	"shipping-service/models"
	"shipping-service/services"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// ---- mock repository ----

type mockShipmentRepo struct {
	createErr              error
	findByIDShipment       *models.Shipment
	findByIDErr            error
	findByOrderIDShipment  *models.Shipment
	findByOrderIDErr       error
	findByTrackingShipment *models.Shipment
	findByTrackingErr      error
	updateErr              error
}

func (m *mockShipmentRepo) Create(_ context.Context, s *models.Shipment) error {
	return m.createErr
}
func (m *mockShipmentRepo) FindByID(_ context.Context, id uuid.UUID) (*models.Shipment, error) {
	return m.findByIDShipment, m.findByIDErr
}
func (m *mockShipmentRepo) FindByOrderID(_ context.Context, _ string) (*models.Shipment, error) {
	return m.findByOrderIDShipment, m.findByOrderIDErr
}
func (m *mockShipmentRepo) FindByTrackingCode(_ context.Context, _ string) (*models.Shipment, error) {
	return m.findByTrackingShipment, m.findByTrackingErr
}
func (m *mockShipmentRepo) Update(_ context.Context, s *models.Shipment) error {
	return m.updateErr
}
func (m *mockShipmentRepo) FindAll(_ context.Context, _, _ int) ([]models.Shipment, int64, error) {
	return nil, 0, nil
}

// ---- mock provider ----

type mockProvider struct {
	rates     []models.ShippingRate
	ratesErr  error
	trackInfo models.TrackingInfo
	labelErr  error
	tracking  models.TrackingStatus
	trackErr  error
}

func (m *mockProvider) GetRates(_, _ interface{}, _ models.Address) ([]models.ShippingRate, error) {
	return nil, nil
}
func (m *mockProvider) CreateLabel(_ models.CreateLabelRequest) (models.TrackingInfo, error) {
	return m.trackInfo, m.labelErr
}
func (m *mockProvider) TrackShipment(_, _ string) (models.TrackingStatus, error) {
	return m.tracking, m.trackErr
}

// GetRates with correct signature
type typedMockProvider struct {
	rates    []models.ShippingRate
	ratesErr error
	info     models.TrackingInfo
	labelErr error
	status   models.TrackingStatus
	trackErr error
}

func (p *typedMockProvider) GetRates(weightKg float64, origin, dest models.Address) ([]models.ShippingRate, error) {
	return p.rates, p.ratesErr
}
func (p *typedMockProvider) CreateLabel(req models.CreateLabelRequest) (models.TrackingInfo, error) {
	return p.info, p.labelErr
}
func (p *typedMockProvider) TrackShipment(carrier, code string) (models.TrackingStatus, error) {
	return p.status, p.trackErr
}

// ---- mock SNS publisher ----

type mockSNS struct{ publishErr error }

func (m *mockSNS) Publish(_ context.Context, _ string, _ []byte) error { return m.publishErr }

// ---- helper ----

func newTestService(repo *mockShipmentRepo, provider *typedMockProvider, sns *mockSNS) services.ShippingService {
	logger, _ := zap.NewDevelopment()
	origin := models.Address{Name: "Warehouse", Street1: "1 W St", City: "SF", State: "CA", PostalCode: "94105", Country: "US"}
	return services.NewShippingService(repo, provider, sns, "arn:aws:sns:us-east-1:000000000000:shipping", origin, logger)
}

// ---- tests ----

func TestGetRates_Success(t *testing.T) {
	repo := &mockShipmentRepo{}
	provider := &typedMockProvider{
		rates: []models.ShippingRate{{Provider: "USPS", Amount: 8.50, Currency: "USD"}},
	}
	svc := newTestService(repo, provider, &mockSNS{})

	req := &models.ShippingRatesRequest{WeightKg: 1.0, Destination: models.Address{Country: "US"}}
	rates, err := svc.GetRates(context.Background(), req)

	assert.Nil(t, err)
	assert.Len(t, rates, 1)
	assert.Equal(t, "USPS", rates[0].Provider)
}

func TestGetRates_ProviderError(t *testing.T) {
	repo := &mockShipmentRepo{}
	provider := &typedMockProvider{ratesErr: errors.New("provider offline")}
	svc := newTestService(repo, provider, &mockSNS{})

	_, svcErr := svc.GetRates(context.Background(), &models.ShippingRatesRequest{WeightKg: 1.0})
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 502, se.StatusCode)
	}
}

func TestGetRates_EmptyRates(t *testing.T) {
	repo := &mockShipmentRepo{}
	provider := &typedMockProvider{rates: []models.ShippingRate{}}
	svc := newTestService(repo, provider, &mockSNS{})

	_, svcErr := svc.GetRates(context.Background(), &models.ShippingRatesRequest{WeightKg: 0.5})
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 404, se.StatusCode)
	}
}

func TestCreateLabel_Success(t *testing.T) {
	repo := &mockShipmentRepo{
		findByOrderIDErr: errors.New("not found"),
	}
	provider := &typedMockProvider{
		info: models.TrackingInfo{
			TrackingCode:   "1Z123",
			LabelURL:       "https://ship.po/label.pdf",
			TrackingURL:    "https://usps.com/track",
			ShippoObjectID: "obj_123",
		},
	}
	svc := newTestService(repo, provider, &mockSNS{})

	req := &models.CreateLabelRequest{OrderID: "o1", UserID: "u1", RateID: "rate_1", WeightKg: 2.0}
	shipment, svcErr := svc.CreateLabel(context.Background(), req)
	assert.Nil(t, svcErr)
	assert.NotNil(t, shipment)
	assert.Equal(t, "1Z123", shipment.TrackingCode)
}

func TestCreateLabel_DuplicateOrder(t *testing.T) {
	existing := &models.Shipment{OrderID: "o2", Status: models.ShipmentStatusCreated}
	repo := &mockShipmentRepo{
		findByOrderIDShipment: existing,
	}
	provider := &typedMockProvider{}
	svc := newTestService(repo, provider, &mockSNS{})

	req := &models.CreateLabelRequest{OrderID: "o2", UserID: "u2", RateID: "rate_2", WeightKg: 1.0}
	shipment, svcErr := svc.CreateLabel(context.Background(), req)
	assert.Nil(t, svcErr)
	assert.Equal(t, existing, shipment)
}

func TestCreateLabel_ProviderError(t *testing.T) {
	repo := &mockShipmentRepo{findByOrderIDErr: errors.New("not found")}
	provider := &typedMockProvider{labelErr: errors.New("carrier rejected")}
	svc := newTestService(repo, provider, &mockSNS{})

	_, svcErr := svc.CreateLabel(context.Background(), &models.CreateLabelRequest{OrderID: "o3", UserID: "u3", RateID: "r3", WeightKg: 0.5})
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 502, se.StatusCode)
	}
}

func TestTrackShipment_Success(t *testing.T) {
	existing := &models.Shipment{ID: uuid.New(), OrderID: "o4", Carrier: "USPS", Status: models.ShipmentStatusCreated}
	repo := &mockShipmentRepo{findByTrackingShipment: existing}
	provider := &typedMockProvider{
		status: models.TrackingStatus{TrackingCode: "TRK1", Status: models.ShipmentStatusInTransit, Carrier: "USPS"},
	}
	svc := newTestService(repo, provider, &mockSNS{})

	status, svcErr := svc.TrackShipment(context.Background(), "TRK1")
	assert.Nil(t, svcErr)
	assert.Equal(t, models.ShipmentStatusInTransit, status.Status)
}

func TestTrackShipment_ProviderError(t *testing.T) {
	repo := &mockShipmentRepo{findByTrackingErr: errors.New("not found")}
	provider := &typedMockProvider{trackErr: errors.New("carrier down")}
	svc := newTestService(repo, provider, &mockSNS{})

	_, svcErr := svc.TrackShipment(context.Background(), "TRK_FAIL")
	assert.NotNil(t, svcErr)
	var se *services.ServiceError
	if assert.ErrorAs(t, svcErr, &se) {
		assert.Equal(t, 502, se.StatusCode)
	}
}
