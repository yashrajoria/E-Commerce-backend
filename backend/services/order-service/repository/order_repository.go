package repositories

import (
	"context"
	"errors"
	"order-service/models"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrStatusConflict is returned by UpdateOrderStatus when the order's current
// status does not match the expected from_status (optimistic locking failure).
var ErrStatusConflict = errors.New("order status conflict: current status does not match expected from_status")

// OrderRepository defines the interface for order data access
type OrderRepository interface {
	FindByUserID(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.Order, int64, error)
	FindAll(ctx context.Context, page, limit int) ([]models.Order, int64, error)
	FindByID(ctx context.Context, orderID uuid.UUID) (*models.Order, error)
	FindByIDAndUserID(ctx context.Context, order_id, userID uuid.UUID) (*models.Order, error)
	Create(ctx context.Context, order *models.Order) error
	Update(ctx context.Context, order *models.Order) error
	// UpdateOrderStatus transitions an order's status from fromStatus to toStatus.
	// It uses an optimistic-locking WHERE clause (AND status = fromStatus) so that
	// concurrent updates on the same order surface as ErrStatusConflict rather than
	// silently overwriting each other.
	UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, fromStatus, toStatus string) error
	FindByIdempotencyKey(ctx context.Context, key string) (*models.Order, error)
	GetRevenueAnalytics(ctx context.Context) (map[string]interface{}, error)
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

func (r *GormOrderRepository) FindByID(ctx context.Context, orderID uuid.UUID) (*models.Order, error) {
	var order models.Order
	if err := r.db.WithContext(ctx).Preload("OrderItems").Where("id = ?", orderID).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
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

func (r *GormOrderRepository) FindByIdempotencyKey(ctx context.Context, key string) (*models.Order, error) {
	var order models.Order
	err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (r *GormOrderRepository) GetRevenueAnalytics(ctx context.Context) (map[string]interface{}, error) {
	var totalRevenue int64
	var revenueToday, revenueYesterday int64
	var countToday, countYesterday int64

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfYesterday := startOfToday.AddDate(0, 0, -1)

	// Lifetime Total
	r.db.WithContext(ctx).Model(&models.Order{}).
		Where("status NOT IN ('CANCELLED', 'REFUNDED', 'canceled', 'refunded')").
		Select("SUM(amount)").Row().Scan(&totalRevenue)

	// Today
	r.db.WithContext(ctx).Model(&models.Order{}).
		Where("status NOT IN ('CANCELLED', 'REFUNDED', 'canceled', 'refunded') AND created_at >= ?", startOfToday).
		Select("SUM(amount), COUNT(*)").Row().Scan(&revenueToday, &countToday)

	// Yesterday
	r.db.WithContext(ctx).Model(&models.Order{}).
		Where("status NOT IN ('CANCELLED', 'REFUNDED', 'canceled', 'refunded') AND created_at >= ? AND created_at < ?", startOfYesterday, startOfToday).
		Select("SUM(amount), COUNT(*)").Row().Scan(&revenueYesterday, &countYesterday)

	return map[string]interface{}{
		"total_revenue":            totalRevenue,
		"revenue_today":            revenueToday,
		"revenue_yesterday":        revenueYesterday,
		"total_orders_today":       countToday,
		"total_orders_yesterday":   countYesterday,
	}, nil
}
