package services

import (
	"context"
	"testing"

	"auth-service/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestLogin_LocksAccountAfterMaxAttempts(t *testing.T) {
	repo := newMemUserRepo()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	repo.users["u@example.com"] = &models.User{
		ID:            uuid.New(),
		Email:         "u@example.com",
		Password:      string(hashed),
		EmailVerified: true,
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	for i := 0; i < maxLoginAttempts; i++ {
		if _, err := svc.Login(context.Background(), "u@example.com", "wrong-password"); err == nil {
			t.Fatalf("attempt %d: expected error for wrong password", i)
		}
	}

	// The account should now be locked, even with the correct password.
	_, err := svc.Login(context.Background(), "u@example.com", "correct-horse")
	if err == nil {
		t.Fatal("expected account to be locked after max failed attempts")
	}
}

func TestLogin_SuccessResetsFailedAttempts(t *testing.T) {
	repo := newMemUserRepo()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	repo.users["u@example.com"] = &models.User{
		ID:            uuid.New(),
		Email:         "u@example.com",
		Password:      string(hashed),
		EmailVerified: true,
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	// A couple of failed attempts, then a successful login.
	_, _ = svc.Login(context.Background(), "u@example.com", "wrong-1")
	_, _ = svc.Login(context.Background(), "u@example.com", "wrong-2")
	if _, err := svc.Login(context.Background(), "u@example.com", "correct-horse"); err != nil {
		t.Fatalf("expected successful login, got %v", err)
	}

	if got := repo.users["u@example.com"].LoginAttempts; got != 0 {
		t.Fatalf("expected LoginAttempts reset to 0, got %d", got)
	}
}

func TestVerifyEmail_CodeIsHashedAtRest(t *testing.T) {
	repo := newMemUserRepo()
	_ = repo.Create(context.Background(), &models.User{
		ID:               uuid.New(),
		Email:            "v@example.com",
		VerificationCode: hashVerificationCode("123456"),
	})
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if repo.users["v@example.com"].VerificationCode == "123456" {
		t.Fatal("verification code stored in plaintext")
	}
	if err := svc.VerifyEmail(context.Background(), "v@example.com", "123456"); err != nil {
		t.Fatalf("expected plaintext code to verify against stored hash, got %v", err)
	}
}
