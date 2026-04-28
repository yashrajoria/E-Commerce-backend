package repository

import (
	"context"
	"errors"
	"promotion-service/models"
	"strings"

	"gorm.io/gorm"
)

var (
	ErrUsageLimitReached = errors.New("coupon usage limit reached")
)

// CouponRepository defines the interface for coupon data access.
type CouponRepository interface {
	Create(ctx context.Context, coupon *models.Coupon) error
	FindByCode(ctx context.Context, code string) (*models.Coupon, error)
	IncrementUsedCount(ctx context.Context, code string) error
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

// IncrementUsedCount atomically increments the used_count of a coupon if the limit hasn't been reached.
func (r *GormCouponRepository) IncrementUsedCount(ctx context.Context, code string) error {
	result := r.db.WithContext(ctx).
		Model(&models.Coupon{}).
		Where("LOWER(code) = ? AND (usage_limit = 0 OR used_count < usage_limit)", strings.ToLower(code)).
		UpdateColumn("used_count", gorm.Expr("used_count + 1"))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrUsageLimitReached
	}

	return nil
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
