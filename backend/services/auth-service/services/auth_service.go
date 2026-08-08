package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"auth-service/models"
	"auth-service/repository"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"go.uber.org/zap"
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
	CountByRole(ctx context.Context, role string) (int64, error)
}

type ITokenService interface {
	GenerateTokenPair(userID, email, role string) (*TokenPair, string, error)
	ValidateToken(tokenStr, expectedType string) (jwt.MapClaims, error)
}

// IEventPublisher publishes domain events to SNS
// notification-service consumes these and handles the actual sending
type IEventPublisher interface {
	Publish(ctx context.Context, eventType string, payload map[string]interface{}) error
}

// dummyPasswordHash is a bcrypt hash with no known plaintext, compared against
// on login when the email lookup fails so the response timing for "unknown
// email" matches "wrong password" and doesn't leak account existence.
const dummyPasswordHash = "$2a$10$CwTycUXWue0Thq9StjUM0uJ8vFN/eD3ubjLBQZTV5D0RwaP0kBRt2"

// verificationCodeTTL is how long an email verification code remains valid.
const verificationCodeTTL = 15 * time.Minute

type AuthService struct {
	userRepo       IUserRepository
	tokenService   ITokenService
	eventPublisher IEventPublisher
	db             *gorm.DB
}

func NewAuthService(
	ur IUserRepository,
	ts ITokenService,
	ep IEventPublisher,
	db *gorm.DB,
) *AuthService {
	return &AuthService{
		userRepo:       ur,
		tokenService:   ts,
		eventPublisher: ep,
		db:             db,
	}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		// Run the hash comparison anyway against a dummy hash so responses for
		// unknown emails take the same time as responses for wrong passwords.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(password))
		return nil, fmt.Errorf("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Only reveal unverified status *after* the password has been confirmed,
	// so an attacker can't use this endpoint to enumerate unverified accounts
	// without knowing the password.
	if !user.EmailVerified {
		return nil, fmt.Errorf("email not verified")
	}

	tokenPair, refreshTokenID, err := s.tokenService.GenerateTokenPair(
		user.ID.String(), user.Email, user.Role,
	)
	if err != nil {
		return nil, err
	}

	rt := &models.RefreshToken{
		TokenID:   refreshTokenID,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.userRepo.CreateRefreshToken(ctx, rt); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return tokenPair, nil
}

func (s *AuthService) Register(ctx context.Context, name, email, password, role string) error {
	return s.ProvisionUser(ctx, ProvisionUserInput{
		Name:          name,
		Email:         email,
		Password:      password,
		Role:          role,
		EmailVerified: false,
		Notify:        true,
	})
}

// ProvisionUserInput configures account creation for public signup, admin invite, or bootstrap.
type ProvisionUserInput struct {
	Name          string
	Email         string
	Password      string
	Role          string
	EmailVerified bool
	Notify        bool
}

// ProvisionUser creates a user with explicit verification/notification behavior.
func (s *AuthService) ProvisionUser(ctx context.Context, in ProvisionUserInput) error {
	role := in.Role
	if role == "" {
		role = "user"
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		txRepo := repository.NewUserRepository(tx)

		_, err := txRepo.FindByEmail(ctx, in.Email)
		if err == nil {
			return fmt.Errorf("email already exists")
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash password")
		}

		verificationCode := ""
		var verificationCodeExpiresAt time.Time
		if !in.EmailVerified {
			verificationCode = GenerateRandomCode(6)
			verificationCodeExpiresAt = time.Now().Add(verificationCodeTTL)
		}

		newUser := &models.User{
			ID:                        uuid.New(),
			Email:                     in.Email,
			Name:                      in.Name,
			Password:                  string(hashedPassword),
			Role:                      role,
			StoreName:                 "",
			EmailVerified:             in.EmailVerified,
			VerificationCode:          verificationCode,
			VerificationCodeExpiresAt: verificationCodeExpiresAt,
		}

		if err := txRepo.Create(ctx, newUser); err != nil {
			return fmt.Errorf("failed to create account: %w", err)
		}

		if in.Notify && !in.EmailVerified && s.eventPublisher != nil {
			if err := s.eventPublisher.Publish(ctx, "user_registered", map[string]interface{}{
				"email":             newUser.Email,
				"name":              newUser.Name,
				"verification_code": newUser.VerificationCode,
			}); err != nil {
				fmt.Printf("failed to publish user_registered event: %v\n", err)
			}
		}

		return nil
	})
}

// BootstrapAdminFromEnv creates the first admin when none exist.
// Requires ADMIN_EMAIL and ADMIN_PASSWORD. Idempotent and safe for restarts.
// Returns nil when skipped (env unset or admin already present).
func (s *AuthService) BootstrapAdminFromEnv(ctx context.Context) error {
	email := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	name := strings.TrimSpace(os.Getenv("ADMIN_NAME"))
	if name == "" {
		name = "Administrator"
	}

	if email == "" || password == "" {
		zap.L().Info("Admin bootstrap skipped (set ADMIN_EMAIL and ADMIN_PASSWORD to enable)")
		return nil
	}

	count, err := s.userRepo.CountByRole(ctx, "admin")
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count > 0 {
		zap.L().Info("Admin bootstrap skipped (admin already exists)", zap.Int64("admin_count", count))
		return nil
	}

	if err := NewPasswordValidator().ValidatePassword(password); err != nil {
		return fmt.Errorf("ADMIN_PASSWORD does not meet policy: %w", err)
	}

	if err := s.ProvisionUser(ctx, ProvisionUserInput{
		Name:          name,
		Email:         email,
		Password:      password,
		Role:          "admin",
		EmailVerified: true,
		Notify:        false,
	}); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}

	zap.L().Info("Bootstrapped initial admin user", zap.String("email", email))
	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, email, code string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if user.VerificationCode == "" || user.VerificationCode != code {
		return fmt.Errorf("invalid verification code")
	}

	if time.Now().After(user.VerificationCodeExpiresAt) {
		return fmt.Errorf("verification code has expired, please request a new one")
	}

	user.EmailVerified = true
	user.VerificationCode = ""
	user.VerificationCodeExpiresAt = time.Time{}

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

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Always re-read role from DB so promotions/demotions apply on refresh.
	role := user.Role
	email := user.Email
	if email == "" {
		if e, ok := claims["email"].(string); ok {
			email = e
		}
	}

	if existingToken.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("refresh token revoked or expired")
	}

	if err := s.userRepo.RevokeRefreshTokenByTokenID(ctx, tokenIDStr); err != nil {
		return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
	}

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

func (s *AuthService) ResendVerificationEmail(ctx context.Context, email string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	if user.EmailVerified {
		return fmt.Errorf("email already verified")
	}

	verificationCode := GenerateRandomCode(6)
	user.VerificationCode = verificationCode
	user.VerificationCodeExpiresAt = time.Now().Add(verificationCodeTTL)

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update verification code: %w", err)
	}

	// Publish verification resend event; non-fatal on publish failure.
	if s.eventPublisher != nil {
		if err := s.eventPublisher.Publish(ctx, "user_registered", map[string]interface{}{
			"email":             user.Email,
			"name":              user.Name,
			"verification_code": verificationCode,
		}); err != nil {
			fmt.Printf("failed to publish user_registered event for resend: %v\n", err)
		}
	} else {
		fmt.Printf("event publisher unavailable; skipping resend event for %s\n", user.Email)
	}

	return nil
}

// GetUserByID returns the user for status / RBAC checks.
func (s *AuthService) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id")
	}
	return s.userRepo.FindByID(ctx, id)
}
