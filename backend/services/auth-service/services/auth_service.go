package services

import (
	"context"
	"fmt"

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
}

type ITokenService interface {
	GenerateTokenPair(userID, email, role string) (*TokenPair, error)
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

	return s.tokenService.GenerateTokenPair(user.ID.String(), user.Email, user.Role)
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

	return s.tokenService.GenerateTokenPair(userIDStr, email, role)
}
