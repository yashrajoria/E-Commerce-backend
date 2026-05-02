package services

import (
	"context"
	"user-service/models"
	"user-service/repository"

	"github.com/yashrajoria/common/logger"
	"go.uber.org/zap"
)

// UserService handles business logic for user operations
type UserService struct {
	repo repository.UserRepository
}

// NewUserService creates a new UserService
func NewUserService(repo repository.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) GetUserProfile(ctx context.Context, userID string) (*models.User, error) {
	logger.Info(ctx, "Fetching user profile", zap.String("user_id", userID))
	return s.repo.GetByID(ctx, userID)
}

func (s *UserService) UpdateUserProfile(ctx context.Context, userID string, name *string, phoneNumber *string) (*models.User, error) {
	logger.Info(ctx, "Updating user profile", zap.String("user_id", userID))
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if name != nil {
		user.Name = *name
	}
	if phoneNumber != nil {
		user.PhoneNumber = phoneNumber
	}

	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	logger.Info(ctx, "User profile updated successfully", zap.String("user_id", userID))
	return user, nil
}

func (s *UserService) ChangeUserPassword(ctx context.Context, userID string, hashedPassword string) error {
	logger.Info(ctx, "Changing user password", zap.String("user_id", userID))
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	user.Password = hashedPassword
	err = s.repo.Update(ctx, user)
	if err == nil {
		logger.Info(ctx, "User password changed successfully", zap.String("user_id", userID))
	}
	return err
}

func (s *UserService) ListUsers(ctx context.Context, page, pageSize int) ([]models.User, int64, error) {
	logger.Info(ctx, "Listing users", zap.Int("page", page), zap.Int("page_size", pageSize))
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, offset, pageSize)
}
