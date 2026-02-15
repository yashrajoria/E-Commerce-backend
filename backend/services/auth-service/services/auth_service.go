package services

import (
	"context"
	"fmt"
	"time"

	"auth-service/models"
	"auth-service/repository"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type IUserRepository interface {
	FindByEmail(ctx context.Context, email string) (*models.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
	Update(ctx context.Context, user *models.User) error
	CreateRefreshToken(ctx context.Context, rt *models.RefreshToken) error
	GetRefreshTokenByTokenID(ctx context.Context, tokenID string) (*models.RefreshToken, error)
	RevokeRefreshTokenByTokenID(ctx context.Context, tokenID string) error
}

type ITokenService interface {
	GenerateTokenPair(userID, email, role string) (*TokenPair, string, error)
	ValidateToken(tokenStr, expectedType string) (jwt.MapClaims, error)
}

type IEmailService interface {
	SendVerificationEmail(email, code string) error
}

// Placeholder for a real email service
type EmailService struct{}

func NewEmailService() *EmailService { return &EmailService{} }

type AuthService struct {
	userRepo     IUserRepository
	tokenService ITokenService
	db           *gorm.DB
}

func NewAuthService(ur IUserRepository, ts ITokenService, db *gorm.DB) *AuthService {
	return &AuthService{userRepo: ur, tokenService: ts, db: db}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	if !user.EmailVerified {
		return nil, fmt.Errorf("email not verified")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	tokenPair, refreshTokenID, err := s.tokenService.GenerateTokenPair(user.ID.String(), user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	// persist refresh token for rotation/revocation
	rt := &models.RefreshToken{
		TokenID:   refreshTokenID,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.userRepo.CreateRefreshToken(ctx, rt); err != nil {
		// if persistence fails, do not return tokens
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return tokenPair, nil
}

func (s *AuthService) Register(ctx context.Context, name, email, password, role string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		txRepo := repository.NewUserRepository(tx)

		_, err := txRepo.FindByEmail(ctx, email)
		if err == nil {
			return fmt.Errorf("email already exists")
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password")
		}

		verificationCode := GenerateRandomCode(6)
		newUser := &models.User{
			ID:               uuid.New(),
			Email:            email,
			Name:             name,
			Password:         string(hashedPassword),
			Role:             role,
			StoreName:        "",
			EmailVerified:    false,
			VerificationCode: verificationCode,
		}

		if err := txRepo.Create(ctx, newUser); err != nil {
			return fmt.Errorf("failed to create account: %w", err)
		}

		if err := SendVerificationEmail(newUser.Email, newUser.VerificationCode); err != nil {
			return fmt.Errorf("failed to send verification email: %w", err)
		}

		return nil
	})
}

func (s *AuthService) VerifyEmail(ctx context.Context, email, code string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if user.VerificationCode != code {
		return fmt.Errorf("invalid verification code")
	}

	user.EmailVerified = true
	user.VerificationCode = ""

	return s.userRepo.Update(ctx, user)
}

func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.tokenService.ValidateToken(refreshToken, "refresh")
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	userIDStr, ok := claims["sub"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid token: user ID (sub) claim is missing or not a string")
	}

	// Verify refresh token hasn't been revoked
	tokenIDStr, ok := claims["jti"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid token: jti claim missing")
	}

	existingToken, err := s.userRepo.GetRefreshTokenByTokenID(ctx, tokenIDStr)
	if err != nil {
		return nil, fmt.Errorf("refresh token not found or invalid")
	}

	if existingToken.Revoked {
		return nil, fmt.Errorf("refresh token has been revoked")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid token: user ID (sub) is not a valid UUID")
	}

	_, err = s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	email, ok := claims["email"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid token: email claim is missing or not a string")
	}
	role, ok := claims["role"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid token: role claim is missing or not a string")
	}

	// verify the refresh token against DB (jti)
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return nil, fmt.Errorf("invalid refresh token: jti missing")
	}

	stored, err := s.userRepo.GetRefreshTokenByTokenID(ctx, jti)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	if stored.Revoked || stored.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("refresh token revoked or expired")
	}

	// revoke old refresh token
	if err := s.userRepo.RevokeRefreshTokenByTokenID(ctx, jti); err != nil {
		return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
	}

	// generate new pair and persist new refresh token
	tokenPair, newTokenID, err := s.tokenService.GenerateTokenPair(userIDStr, email, role)
	if err != nil {
		return nil, err
	}

	newRT := &models.RefreshToken{
		TokenID:   newTokenID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.userRepo.CreateRefreshToken(ctx, newRT); err != nil {
		return nil, fmt.Errorf("failed to store new refresh token: %w", err)
	}

	return tokenPair, nil
}

// Logout revokes a given refresh token so it cannot be used anymore
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokenService.ValidateToken(refreshToken, "refresh")
	if err != nil {
		return err
	}
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return fmt.Errorf("invalid refresh token: jti missing")
	}
	return s.userRepo.RevokeRefreshTokenByTokenID(ctx, jti)
}

// ResendVerificationEmail generates a new verification code and sends it to the user
func (s *AuthService) ResendVerificationEmail(ctx context.Context, email string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if user.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	// Generate a new verification code
	verificationCode := GenerateRandomCode(6)
	user.VerificationCode = verificationCode

	// Update user with new code
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update verification code: %w", err)
	}

	// Send verification email
	if err := SendVerificationEmail(user.Email, verificationCode); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	return nil
}
