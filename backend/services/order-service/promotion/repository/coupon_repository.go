package repository

import (
	"context"
	"errors"
	"order-service/promotion/models"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrUsageLimitReached = errors.New("coupon usage limit reached")
)

// CouponRepository defines the interface for coupon data access.
type CouponRepository interface {
	Create(ctx context.Context, coupon *models.Coupon) error
	FindByCode(ctx context.Context, code string) (*models.Coupon, error)
	IncrementUsedCount(ctx context.Context, code string, orderID, userID uuid.UUID) error
	Deactivate(ctx context.Context, code string) error
	FindAll(ctx context.Context, page, limit int) ([]models.Coupon, int64, error)
}

// GormCouponRepository implements CouponRepository using GORM.
type GormCouponRepository struct {
	db *gorm.DB
}

// NewGormCouponRepository creates a new GormCouponRepository.
func NewGormCouponRepository(db *gorm.DB) CouponRepository {
	return &GormCouponRepository{db: db}
}

// Create inserts a new coupon into the database.
func (r *GormCouponRepository) Create(ctx context.Context, coupon *models.Coupon) error {
	return r.db.WithContext(ctx).Create(coupon).Error
}

// FindByCode retrieves an active coupon by its code (case-insensitive).
func (r *GormCouponRepository) FindByCode(ctx context.Context, code string) (*models.Coupon, error) {
	var coupon models.Coupon
	err := r.db.WithContext(ctx).
		Where("LOWER(code) = ? AND active = ?", strings.ToLower(code), true).
		First(&coupon).Error
	if err != nil {
		return nil, err
	}
	return &coupon, nil
}

// IncrementUsedCount records the redemption (one per order) and increments
// used_count if the limit hasn't been reached, in one transaction. A redelivered
// order_created event hits the coupon_usages unique(order_id) and is a no-op.
func (r *GormCouponRepository) IncrementUsedCount(ctx context.Context, code string, orderID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ins := tx.Exec(
			`INSERT INTO coupon_usages (coupon_id, order_id, user_id)
			 SELECT id, ?, ? FROM coupons WHERE LOWER(code) = ? AND deleted_at IS NULL
			 ON CONFLICT (order_id) DO NOTHING`,
			orderID, userID, strings.ToLower(code))
		if ins.Error != nil {
			return ins.Error
		}
		if ins.RowsAffected == 0 {
			return nil // already redeemed for this order
		}

		result := tx.Model(&models.Coupon{}).
			Where("LOWER(code) = ? AND (usage_limit = 0 OR used_count < usage_limit)", strings.ToLower(code)).
			UpdateColumn("used_count", gorm.Expr("used_count + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrUsageLimitReached // rolls back the usage row
		}
		return nil
	})
}

// Deactivate soft-deactivates a coupon by setting active = false.
func (r *GormCouponRepository) Deactivate(ctx context.Context, code string) error {
	result := r.db.WithContext(ctx).
		Model(&models.Coupon{}).
		Where("LOWER(code) = ?", strings.ToLower(code)).
		Update("active", false)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindAll retrieves paginated coupons.
func (r *GormCouponRepository) FindAll(ctx context.Context, page, limit int) ([]models.Coupon, int64, error) {
	var coupons []models.Coupon
	var total int64

	query := r.db.WithContext(ctx).Model(&models.Coupon{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.
		Offset(offset).
		Limit(limit).
		Order("created_at DESC").
		Find(&coupons).Error; err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}
