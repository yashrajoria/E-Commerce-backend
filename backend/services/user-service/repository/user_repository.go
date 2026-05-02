package repository

import (
	"context"
	"user-service/models"
	"github.com/yashrajoria/common/logger"

	"gorm.io/gorm"
	"go.uber.org/zap"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	GetByID(ctx context.Context, id string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	List(ctx context.Context, offset, limit int) ([]models.User, int64, error)
}

// GormUserRepository is the GORM implementation of UserRepository
type GormUserRepository struct {
	db *gorm.DB
}

// NewGormUserRepository creates a new GormUserRepository
func NewGormUserRepository(db *gorm.DB) *GormUserRepository {
	return &GormUserRepository{db: db}
}

func (r *GormUserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&user).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			logger.Error(ctx, "Failed to get user by ID", err, zap.String("user_id", id))
		}
		return nil, err
	}
	return &user, nil
}

func (r *GormUserRepository) Update(ctx context.Context, user *models.User) error {
	err := r.db.WithContext(ctx).Save(user).Error
	if err != nil {
		logger.Error(ctx, "Failed to update user", err, zap.String("user_id", user.ID.String()))
		return err
	}
	return nil
}

func (r *GormUserRepository) List(ctx context.Context, offset, limit int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	if err := r.db.WithContext(ctx).Model(&models.User{}).Count(&total).Error; err != nil {
		logger.Error(ctx, "Failed to count users", err)
		return nil, 0, err
	}

	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&users).Error

	if err != nil {
		logger.Error(ctx, "Failed to list users", err, zap.Int("offset", offset), zap.Int("limit", limit))
		return nil, 0, err
	}

	return users, total, nil
}
