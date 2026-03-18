package services

import (
	"context"
	"user-service/models"
	"user-service/repository"
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
	return s.repo.GetByID(ctx, userID)
}

func (s *UserService) UpdateUserProfile(ctx context.Context, userID string, name *string, phoneNumber *string) (*models.User, error) {
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
	return user, nil
}

func (s *UserService) ChangeUserPassword(ctx context.Context, userID string, hashedPassword string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	user.Password = hashedPassword
	return s.repo.Update(ctx, user)
}

func (s *UserService) ListUsers(ctx context.Context, page, pageSize int) ([]models.User, int64, error) {
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, offset, pageSize)
}
