package repository

import (
	"context"
	"time"

	"identity-service/models"

	"github.com/google/uuid"
	"github.com/yashrajoria/common/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// UserRepository is the single data-access implementation for identity.
// One struct satisfies both the auth flows (credentials, refresh tokens) and
// the profile flows (lookup, update, list) — previously two structs in two
// services over the same tables.
type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// --- Auth flows (ex-auth-service) ---

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	return &user, err
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	return &user, err
}

// Refresh token storage
func (r *UserRepository) CreateRefreshToken(ctx context.Context, rt *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(rt).Error
}

func (r *UserRepository) GetRefreshTokenByTokenID(ctx context.Context, tokenID string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	err := r.db.WithContext(ctx).Where("token_id = ?", tokenID).First(&rt).Error
	return &rt, err
}

func (r *UserRepository) RevokeRefreshTokenByTokenID(ctx context.Context, tokenID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).Where("token_id = ?", tokenID).
		Updates(map[string]interface{}{"revoked": true, "revoked_at": now}).Error
}

func (r *UserRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).Where("user_id = ?", userID).Update("revoked", true).Error
}

// RevokeRefreshTokenFamily revokes every token descended from the same login,
// used when a rotated-out token is presented again (reuse => theft signal).
func (r *UserRepository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).Where("family_id = ?", familyID).Update("revoked", true).Error
}

// CountByRole returns how many users have the given role.
func (r *UserRepository) CountByRole(ctx context.Context, role string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

// --- Profile flows (ex-user-service) ---

func (r *UserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
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

func (r *UserRepository) List(ctx context.Context, offset, limit int) ([]models.User, int64, error) {
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
