package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// TestExpiredAccessToken_IsRejected verifies that a JWT whose exp claim is in
// the past is rejected by ValidateToken regardless of whether the signature is
// otherwise valid. This guards against clock-skew or accidental TTL changes
// letting stale tokens through.
func TestExpiredAccessToken_IsRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-bytes-long!!")
	svc := NewTokenService()

	// Build a token that expired 1 second ago with a valid signature.
	claims := jwt.MapClaims{
		"sub":   "11111111-1111-4111-8111-111111111111",
		"email": "test@example.com",
		"role":  "user",
		"typ":   "access",
		"exp":   time.Now().Add(-1 * time.Second).Unix(),
		"iat":   time.Now().Add(-5 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(svc.secretKey)
	if err != nil {
		t.Fatalf("sign expired token: %v", err)
	}

	_, err = svc.ValidateToken(tokenStr, "access")
	if err == nil {
		t.Fatal("expected error for expired access token, got nil")
	}
}

// TestExpiredRefreshToken_IsRejected verifies that an expired refresh token is
// similarly rejected.
func TestExpiredRefreshToken_IsRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-bytes-long!!")
	svc := NewTokenService()

	claims := jwt.MapClaims{
		"sub":   "22222222-2222-4222-8222-222222222222",
		"email": "test2@example.com",
		"role":  "user",
		"typ":   "refresh",
		"jti":   "old-jti",
		"exp":   time.Now().Add(-24 * time.Hour).Unix(),
		"iat":   time.Now().Add(-25 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(svc.secretKey)
	if err != nil {
		t.Fatalf("sign expired refresh token: %v", err)
	}

	_, err = svc.ValidateToken(tokenStr, "refresh")
	if err == nil {
		t.Fatal("expected error for expired refresh token, got nil")
	}
}

// TestWrongTokenType_IsRejected verifies that presenting a refresh token where
// an access token is expected (and vice versa) is rejected.
func TestWrongTokenType_IsRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-bytes-long!!")
	svc := NewTokenService()

	// Mint a valid *refresh* token
	claims := jwt.MapClaims{
		"sub":   "33333333-3333-4333-8333-333333333333",
		"email": "test3@example.com",
		"role":  "user",
		"typ":   "refresh",
		"jti":   "some-jti",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(svc.secretKey)
	if err != nil {
		t.Fatalf("sign refresh token: %v", err)
	}

	// Attempt to validate it as an *access* token — must fail.
	_, err = svc.ValidateToken(tokenStr, "access")
	if err == nil {
		t.Fatal("expected error when presenting refresh token as access token, got nil")
	}
}

// TestTamperedSignature_IsRejected verifies that a token with a modified
// signature (simulating a forged token) is rejected.
func TestTamperedSignature_IsRejected(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret-at-least-32-bytes-long!!")
	svc := NewTokenService()

	pair, _, err := svc.GenerateTokenPair("44444444-4444-4444-8444-444444444444", "t@example.com", "user")
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	// Corrupt the last few bytes of the signature part (third JWT segment).
	tokenStr := pair.AccessToken
	tokenStr = tokenStr[:len(tokenStr)-4] + "XXXX"

	_, err = svc.ValidateToken(tokenStr, "access")
	if err == nil {
		t.Fatal("expected error for token with corrupted signature, got nil")
	}
}
