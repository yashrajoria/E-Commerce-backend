package services

import (
	"auth-service/models"
	"auth-service/repository"
	"context"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Placeholder for a real email service
type EmailService struct{}

func NewEmailService() *EmailService { return &EmailService{} }
func (s *EmailService) SendVerificationEmail(email, code string) error {
	// In a real application, you would use an SMTP client or an API like SendGrid.
	fmt.Printf("--- SIMULATING EMAIL ---\nTo: %s\nVerification Code: %s\n------------------------\n", email, code)
	return nil
}

type AuthService struct {
	userRepo     *repository.UserRepository
	tokenService *TokenService
	emailService *EmailService
	db           *gorm.DB // Inject DB for transaction support
}

func NewAuthService(ur *repository.UserRepository, ts *TokenService, es *EmailService, db *gorm.DB) *AuthService {
	return &AuthService{userRepo: ur, tokenService: ts, emailService: es, db: db}
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

	// CORRECTED LOGIC: Generate a token pair and return it directly.
	return s.tokenService.GenerateTokenPair(user.ID.String(), user.Email, user.Role)
}

func (s *AuthService) Register(ctx context.Context, name, email, password string) error {
	// Use a transaction to ensure user creation and email sending are atomic.
	return s.db.Transaction(func(tx *gorm.DB) error {
		txRepo := repository.NewUserRepository(tx)

		// Check if email already exists
		_, err := txRepo.FindByEmail(ctx, email)
		if err == nil {
			return fmt.Errorf("email already exists")
		}
		if err != gorm.ErrRecordNotFound {
			return err // A real database error occurred
		}

		// Hash password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password")
		}

		verificationCode := "123456" // Replace with a real random code generator

		newUser := &models.User{
			ID:               uuid.New(),
			Email:            email,
			Name:             name,
			Password:         string(hashedPassword),
			Role:             "user", // Role should always be set by the server
			EmailVerified:    false,
			VerificationCode: verificationCode,
		}

		// Create user record within the transaction
		if err := txRepo.Create(ctx, newUser); err != nil {
			return fmt.Errorf("failed to create account: %w", err)
		}

		// Send verification email
		if err := s.emailService.SendVerificationEmail(newUser.Email, newUser.VerificationCode); err != nil {
			// Because we are in a transaction, this error will cause the user creation to be rolled back.
			return fmt.Errorf("failed to send verification email: %w", err)
		}

		return nil // Commit the transaction
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
	user.VerificationCode = "" // Clear the code after successful verification

	return s.userRepo.Update(ctx, user)
}

func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	// 1. Validate the refresh token using the TokenService
	claims, err := s.tokenService.ValidateToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// 2. Business logic: Ensure the user still exists
	userIDStr := claims["sub"].(string)
	userID, _ := uuid.Parse(userIDStr)
	_, err = s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// 3. Generate a new token pair using the TokenService
	email := claims["email"].(string)
	role := claims["role"].(string)
	return s.tokenService.GenerateTokenPair(userIDStr, email, role)
}
