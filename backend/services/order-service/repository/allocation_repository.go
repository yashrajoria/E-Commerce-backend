// Package repository provides database access for orders, order items, and transactional outbox events.
package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"order-service/models"
)

// AllocationRepository defines the interface contract for allocation repository.
type AllocationRepository interface {
	Create(ctx context.Context, allocation *models.OrderPaymentAllocation) error
	FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.OrderPaymentAllocation, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateRefunded(ctx context.Context, id uuid.UUID, refundedAmount int64) error
}

// GormAllocationRepository represents gorm allocation repository.
type GormAllocationRepository struct {
	db *gorm.DB
}

// NewGormAllocationRepository creates and initializes a new GormAllocationRepository instance.
func NewGormAllocationRepository(db *gorm.DB) *GormAllocationRepository {
	return &GormAllocationRepository{db: db}
}

// Create persists a new  record in the database.
func (r *GormAllocationRepository) Create(ctx context.Context, allocation *models.OrderPaymentAllocation) error {
	if allocation.ID == uuid.Nil {
		allocation.ID = uuid.New()
	}
	now := time.Now().UTC()
	allocation.CreatedAt = now
	allocation.UpdatedAt = now
	return r.db.WithContext(ctx).Create(allocation).Error
}

// FindByOrderID queries the database for  by order id.
func (r *GormAllocationRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.OrderPaymentAllocation, error) {
	var allocations []models.OrderPaymentAllocation
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).Order("created_at ASC").Find(&allocations).Error
	return allocations, err
}

// UpdateStatus updates status in the database.
func (r *GormAllocationRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.db.WithContext(ctx).Model(&models.OrderPaymentAllocation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now().UTC(),
		}).Error
}

// UpdateRefunded updates refunded in the database.
func (r *GormAllocationRepository) UpdateRefunded(ctx context.Context, id uuid.UUID, refundedAmount int64) error {
	updates := map[string]interface{}{
		"refunded_amount": refundedAmount,
		"updated_at":      time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Model(&models.OrderPaymentAllocation{}).
		Where("id = ?", id).
		Updates(updates).Error
}
