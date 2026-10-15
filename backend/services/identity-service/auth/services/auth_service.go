package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"identity-service/models"
	"identity-service/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/yashrajoria/common/password"
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
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error
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

	if user.LoginLockedUntil != nil && time.Now().Before(*user.LoginLockedUntil) {
		return nil, fmt.Errorf("too many failed login attempts, try again later")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.recordFailedLogin(ctx, user)
		return nil, fmt.Errorf("invalid email or password")
	}

	if user.LoginAttempts != 0 || user.LoginLockedUntil != nil {
		user.LoginAttempts = 0
		user.LoginLockedUntil = nil
		if updateErr := s.userRepo.Update(ctx, user); updateErr != nil {
			zap.L().Warn("failed to reset login attempt count", zap.Error(updateErr))
		}
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
		FamilyID:  uuid.New(),
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
	in.Role = role

	// Allow unit tests to exercise registration without a *gorm.DB.
	if s.db == nil {
		return s.provisionWithRepo(ctx, s.userRepo, in)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		txRepo := repository.NewUserRepository(tx)
		return s.provisionWithRepo(ctx, txRepo, in)
	})
}

func (s *AuthService) provisionWithRepo(ctx context.Context, repo IUserRepository, in ProvisionUserInput) error {
	_, err := repo.FindByEmail(ctx, in.Email)
	if err == nil {
		return ErrEmailAlreadyExists
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password")
	}

	verificationCode := ""
	if !in.EmailVerified {
		verificationCode = GenerateRandomCode(6)
	}

	newUser := &models.User{
		ID:            uuid.New(),
		Email:         in.Email,
		Name:          in.Name,
		Password:      string(hashedPassword),
		Role:          in.Role,
		StoreName:     "",
		EmailVerified: in.EmailVerified,
	}
	if verificationCode != "" {
		newUser.VerificationCode = hashVerificationCode(verificationCode)
		exp := time.Now().Add(verificationCodeTTL)
		newUser.VerificationExpiresAt = &exp
	}

	if err := repo.Create(ctx, newUser); err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	if in.Notify && !in.EmailVerified && s.eventPublisher != nil {
		if err := s.eventPublisher.Publish(ctx, "user_registered", map[string]interface{}{
			"email": newUser.Email,
			"name":  newUser.Name,
			// Plaintext code — sent once here to be emailed to the user.
			// Only the hash (newUser.VerificationCode) is persisted.
			"verification_code": verificationCode,
		}); err != nil {
			fmt.Printf("failed to publish user_registered event: %v\n", err)
		}
	}

	return nil
}

type GuestSessionInput struct {
	Email string
	Name  string
	Phone string
}

const guestAccountMarker = "GUEST_ACCOUNT"

// GuestSession provisions or retrieves a guest user session for checkout.
// If the email belongs to a registered account with a password, ErrAccountExists is returned.
func (s *AuthService) GuestSession(ctx context.Context, in GuestSessionInput) (*TokenPair, error) {
	email := strings.TrimSpace(strings.ToLower(in.Email))
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Guest Customer"
	}

	user, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil {
		if user.VerificationCode != guestAccountMarker {
			// Account was registered with a password — require proper login.
			return nil, ErrAccountExists
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		randomPassword := uuid.NewString() + uuid.NewString()
		hashedPassword, hashErr := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, fmt.Errorf("failed to hash password")
		}

		user = &models.User{
			ID:               uuid.New(),
			Email:            email,
			Name:             name,
			Password:         string(hashedPassword),
			Role:             "user",
			EmailVerified:    true,
			VerificationCode: guestAccountMarker,
		}

		if createErr := s.userRepo.Create(ctx, user); createErr != nil {
			return nil, fmt.Errorf("failed to create guest account: %w", createErr)
		}
	} else {
		return nil, err
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
		FamilyID:  uuid.New(),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := s.userRepo.CreateRefreshToken(ctx, rt); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return tokenPair, nil
}

// BootstrapAdminFromEnv creates the first admin when none exist.
// Requires ADMIN_EMAIL and ADMIN_PASSWORD. Idempotent and safe for restarts.
// Returns nil when skipped (env unset or admin already present).
func (s *AuthService) BootstrapAdminFromEnv(ctx context.Context) error {
	email := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_EMAIL")))
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	name := strings.TrimSpace(os.Getenv("ADMIN_NAME"))
	if name == "" {
		name = "Administrator"
	}

	if email == "" || adminPassword == "" {
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

	if err := password.NewValidator().ValidatePassword(adminPassword); err != nil {
		return fmt.Errorf("ADMIN_PASSWORD does not meet policy: %w", err)
	}

	if err := s.ProvisionUser(ctx, ProvisionUserInput{
		Name:          name,
		Email:         email,
		Password:      adminPassword,
		Role:          "admin",
		EmailVerified: true,
		Notify:        false,
	}); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}

	zap.L().Info("Bootstrapped initial admin user", zap.String("email", email))
	return nil
}

const (
	maxVerificationAttempts  = 5
	verificationLockDuration = 15 * time.Minute

	maxLoginAttempts  = 10
	loginLockDuration = 15 * time.Minute
)

// recordFailedLogin increments the failed-login counter and locks the
// account once maxLoginAttempts is reached. Higher-throughput and shorter
// duration than the strict gateway rate limit alone would allow, but bounds
// a distributed attacker who spreads attempts across many IPs.
func (s *AuthService) recordFailedLogin(ctx context.Context, user *models.User) {
	// Atomic increment: a read-modify-write lets parallel guesses lose counts
	// (bypassing lockout) and Save() would overwrite concurrent profile/password changes.
	if s.db != nil {
		err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).UpdateColumns(map[string]any{
			"login_attempts": gorm.Expr("login_attempts + 1"),
			"login_locked_until": gorm.Expr(
				"CASE WHEN login_attempts + 1 >= ? THEN ? ELSE login_locked_until END",
				maxLoginAttempts, time.Now().Add(loginLockDuration)),
		}).Error
		if err != nil {
			zap.L().Warn("failed to persist failed login attempt count", zap.Error(err))
		}
		return
	}
	user.LoginAttempts++
	if user.LoginAttempts >= maxLoginAttempts {
		lockUntil := time.Now().Add(loginLockDuration)
		user.LoginLockedUntil = &lockUntil
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		zap.L().Warn("failed to persist failed login attempt count", zap.Error(err))
	}
}


// GetUserByID returns the user for status / RBAC checks.
func (s *AuthService) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user id")
	}
	return s.userRepo.FindByID(ctx, id)
}
