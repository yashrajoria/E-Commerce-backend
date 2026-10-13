package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"identity-service/models"
	"identity-service/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

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
		// A refresh token is single-use; seeing an already-rotated one again
		// means it was stolen and used out of band. Kill the whole lineage
		// so both the attacker's and the legitimate user's sessions are cut.
		if revokeErr := s.userRepo.RevokeRefreshTokenFamily(ctx, existingToken.FamilyID); revokeErr != nil {
			zap.L().Error("failed to revoke refresh token family after reuse detection",
				zap.String("family_id", existingToken.FamilyID.String()), zap.Error(revokeErr))
		}
		zap.L().Warn("refresh token reuse detected; family revoked",
			zap.String("family_id", existingToken.FamilyID.String()), zap.String("user_id", existingToken.UserID.String()))
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

	// Skip re-revoking an already-revoked token (grace-period replay) — doing so would
	// reset RevokedAt and keep extending the reuse window indefinitely.
	if !existingToken.Revoked {
		if err := s.userRepo.RevokeRefreshTokenByTokenID(ctx, tokenIDStr); err != nil {
			if errors.Is(err, repository.ErrTokenAlreadyRevoked) {
				// Lost a concurrent-rotation race (e.g. two tabs): reject this call only;
				// the winner's new token stays valid.
				return nil, fmt.Errorf("refresh token has been revoked")
			}
			return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
		}
	}

	tokenPair, newTokenID, err := s.tokenService.GenerateTokenPair(userIDStr, email, role)
	if err != nil {
		return nil, err
	}

	newRT := &models.RefreshToken{
		TokenID:   newTokenID,
		UserID:    userID,
		FamilyID:  existingToken.FamilyID,
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
	if err := s.userRepo.RevokeRefreshTokenByTokenID(ctx, jti); err != nil && !errors.Is(err, repository.ErrTokenAlreadyRevoked) {
		return err
	}
	return nil
}

// RevokeUserTokens revokes every outstanding refresh token for a user.
func (s *AuthService) RevokeUserTokens(ctx context.Context, userID string) error {
	id, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}
	return s.userRepo.RevokeAllUserRefreshTokens(ctx, id)
}

