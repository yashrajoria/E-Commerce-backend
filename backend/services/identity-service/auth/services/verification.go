package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"identity-service/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// GenerateRandomCode returns a numeric code of the given length, used for
// email verification. Uses crypto/rand so codes aren't guessable.
func GenerateRandomCode(length int) string {
	code := ""
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// Fallback to 0 in the unlikely event of entropy failure
			code += "0"
			continue
		}
		code += n.String()
	}
	return code
}

// hashVerificationCode hashes a verification code for storage, so a DB read
// alone (backup leak, SQLi elsewhere) doesn't hand over a live, usable code.
// The plaintext code is only ever transmitted once, in the email itself.
func hashVerificationCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// VerifyEmail executes business logic for verify email.
func (s *AuthService) VerifyEmail(ctx context.Context, email, code string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return ErrUserNotFound
	}

	if user.VerificationLockedUntil != nil && time.Now().Before(*user.VerificationLockedUntil) {
		return fmt.Errorf("%w: too many attempts, try again later", ErrInvalidVerificationCode)
	}

	if user.VerificationExpiresAt != nil && time.Now().After(*user.VerificationExpiresAt) {
		return fmt.Errorf("%w: code expired, request a new one", ErrInvalidVerificationCode)
	}

	if subtle.ConstantTimeCompare([]byte(user.VerificationCode), []byte(hashVerificationCode(code))) != 1 {
		if s.db != nil {
			// Atomic for the same reason as recordFailedLogin: parallel guesses must all count.
			err := s.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", user.ID).UpdateColumns(map[string]any{
				"verification_attempts": gorm.Expr("verification_attempts + 1"),
				"verification_locked_until": gorm.Expr(
					"CASE WHEN verification_attempts + 1 >= ? THEN ? ELSE verification_locked_until END",
					maxVerificationAttempts, time.Now().Add(verificationLockDuration)),
			}).Error
			if err != nil {
				zap.L().Warn("failed to persist verification attempt count", zap.Error(err))
			}
			return ErrInvalidVerificationCode
		}
		user.VerificationAttempts++
		if user.VerificationAttempts >= maxVerificationAttempts {
			lockUntil := time.Now().Add(verificationLockDuration)
			user.VerificationLockedUntil = &lockUntil
		}
		if updateErr := s.userRepo.Update(ctx, user); updateErr != nil {
			zap.L().Warn("failed to persist verification attempt count", zap.Error(updateErr))
		}
		return ErrInvalidVerificationCode
	}

	user.EmailVerified = true
	user.VerificationCode = ""
	user.VerificationExpiresAt = nil
	user.VerificationAttempts = 0
	user.VerificationLockedUntil = nil

	return s.userRepo.Update(ctx, user)
}

// ResendVerificationEmail executes business logic for resend verification email.
func (s *AuthService) ResendVerificationEmail(ctx context.Context, email string) error {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return ErrUserNotFound
	}

	if user.EmailVerified {
		return ErrEmailAlreadyVerified
	}

	verificationCode := GenerateRandomCode(6)
	user.VerificationCode = hashVerificationCode(verificationCode)
	exp := time.Now().Add(verificationCodeTTL)
	user.VerificationExpiresAt = &exp
	user.VerificationAttempts = 0
	user.VerificationLockedUntil = nil

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
