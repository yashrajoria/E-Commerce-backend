package repository

import (
	"auth-service/models"
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

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
	// GORM can work directly with the uuid.UUID type.
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
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).Where("token_id = ?", tokenID).Update("revoked", true).Error
}

func (r *UserRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).Where("user_id = ?", userID).Update("revoked", true).Error
}
