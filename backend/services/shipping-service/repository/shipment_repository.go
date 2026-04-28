package repository

import (
	"context"
	"shipping-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ShipmentRepository defines data-access operations for shipments.
type ShipmentRepository interface {
	Create(ctx context.Context, shipment *models.Shipment) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.Shipment, error)
	FindByOrderID(ctx context.Context, orderID string) (*models.Shipment, error)
	FindByTrackingCode(ctx context.Context, trackingCode string) (*models.Shipment, error)
	Update(ctx context.Context, shipment *models.Shipment) error
	FindAll(ctx context.Context, page, limit int) ([]models.Shipment, int64, error)
}

// GormShipmentRepository implements ShipmentRepository using GORM.
type GormShipmentRepository struct {
	db *gorm.DB
}

// NewGormShipmentRepository creates a new GormShipmentRepository.
func NewGormShipmentRepository(db *gorm.DB) ShipmentRepository {
	return &GormShipmentRepository{db: db}
}

func (r *GormShipmentRepository) Create(ctx context.Context, shipment *models.Shipment) error {
	return r.db.WithContext(ctx).Create(shipment).Error
}

func (r *GormShipmentRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Shipment, error) {
	var s models.Shipment
	if err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *GormShipmentRepository) FindByOrderID(ctx context.Context, orderID string) (*models.Shipment, error) {
	var s models.Shipment
	if err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *GormShipmentRepository) FindByTrackingCode(ctx context.Context, trackingCode string) (*models.Shipment, error) {
	var s models.Shipment
	if err := r.db.WithContext(ctx).
		Where("tracking_code = ?", trackingCode).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *GormShipmentRepository) Update(ctx context.Context, shipment *models.Shipment) error {
	return r.db.WithContext(ctx).Save(shipment).Error
}

func (r *GormShipmentRepository) FindAll(ctx context.Context, page, limit int) ([]models.Shipment, int64, error) {
	var shipments []models.Shipment
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Shipment{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.
		Offset(offset).Limit(limit).
		Order("created_at DESC").
		Find(&shipments).Error; err != nil {
		return nil, 0, err
	}

	return shipments, total, nil
}
