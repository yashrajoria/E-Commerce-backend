package repositories

import (
	"context"
	"order-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrderRepository defines the interface for order data access
type OrderRepository interface {
	FindByUserID(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.Order, int64, error)
	FindAll(ctx context.Context, page, limit int) ([]models.Order, int64, error)
	FindByIDAndUserID(ctx context.Context, order_id, userID uuid.UUID) (*models.Order, error)
	Create(ctx context.Context, order *models.Order) error
	Update(ctx context.Context, order *models.Order) error
}

// GormOrderRepository implements OrderRepository using GORM
type GormOrderRepository struct {
	db *gorm.DB
}

// NewGormOrderRepository creates a new instance of GormOrderRepository
func NewGormOrderRepository(db *gorm.DB) OrderRepository {
	return &GormOrderRepository{db: db}
}

// FindByUserID retrieves orders for a specific user with pagination
func (r *GormOrderRepository) FindByUserID(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.Order, int64, error) {
	var orders []models.Order
	var total int64

	query := r.db.WithContext(ctx).
		Model(&models.Order{}).
		Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.
		Preload("OrderItems").
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// FindAll retrieves all orders with pagination
func (r *GormOrderRepository) FindAll(ctx context.Context, page, limit int) ([]models.Order, int64, error) {
	var orders []models.Order
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Order{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.
		Preload("OrderItems").
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// FindByIDAndUserID retrieves a specific order for a user
func (r *GormOrderRepository) FindByIDAndUserID(ctx context.Context, order_id, userID uuid.UUID) (*models.Order, error) {
	var order models.Order

	if err := r.db.WithContext(ctx).
		Preload("OrderItems").
		Where("id = ? AND user_id = ?", order_id, userID).
		First(&order).Error; err != nil {
		return nil, err
	}

	return &order, nil
}

// Create creates a new order
func (r *GormOrderRepository) Create(ctx context.Context, order *models.Order) error {
	return r.db.WithContext(ctx).Create(order).Error
}

// Update updates an existing order
func (r *GormOrderRepository) Update(ctx context.Context, order *models.Order) error {
	return r.db.WithContext(ctx).Save(order).Error
}
