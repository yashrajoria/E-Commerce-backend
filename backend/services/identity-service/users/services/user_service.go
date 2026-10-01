package services

import (
	"context"
	"identity-service/models"
	"identity-service/repository"

	"github.com/yashrajoria/common/logger"
	"go.uber.org/zap"
)

// tokenRevoker revokes outstanding refresh tokens after a password change.
// In-process implementation: *authservices.AuthService (same binary, same DB
// transaction scope) — the old cross-service HTTP client is gone.
type tokenRevoker interface {
	RevokeUserTokens(ctx context.Context, userID string) error
}

// UserService handles business logic for user operations
type UserService struct {
	repo       *repository.UserRepository
	authClient tokenRevoker
}

// NewUserService creates a new UserService. revoker may be nil (revocation
// becomes a logged no-op); production wiring passes the AuthService.
func NewUserService(repo *repository.UserRepository, revoker tokenRevoker) *UserService {
	return &UserService{repo: repo, authClient: revoker}
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
	if err := s.repo.Update(ctx, user); err != nil {
		return err
	}
	logger.Info(ctx, "User password changed successfully", zap.String("user_id", userID))

	// Revoke every outstanding refresh token so a session stolen before the
	// password change can't keep using it afterward. Best-effort: the
	// password change itself already succeeded, so a revoke failure here is
	// logged, not surfaced as a failed password change (old access tokens
	// still expire naturally within 15 minutes).
	if s.authClient != nil {
		if err := s.authClient.RevokeUserTokens(ctx, userID); err != nil {
			logger.Error(ctx, "Failed to revoke refresh tokens after password change", err, zap.String("user_id", userID))
		}
	}

	return nil
}

func (s *UserService) ListUsers(ctx context.Context, page, pageSize int) ([]models.User, int64, error) {
	logger.Info(ctx, "Listing users", zap.Int("page", page), zap.Int("page_size", pageSize))
	offset := (page - 1) * pageSize
	return s.repo.List(ctx, offset, pageSize)
}
