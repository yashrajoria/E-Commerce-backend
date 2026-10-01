package services

import (
	"context"
	"testing"

	"identity-service/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestRegister_SuccessCreatesVerifiedFalseUser(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if err := svc.Register(context.Background(), "Ada", "ada@example.com", "Str0ng!Pass", "user"); err != nil {
		t.Fatalf("expected register success, got %v", err)
	}
	u, err := repo.FindByEmail(context.Background(), "ada@example.com")
	if err != nil {
		t.Fatalf("expected user persisted, got %v", err)
	}
	if u.EmailVerified {
		t.Fatal("new signup must require email verification")
	}
	if u.VerificationCode == "" {
		t.Fatal("expected hashed verification code to be stored")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte("Str0ng!Pass")); err != nil {
		t.Fatalf("password hash mismatch: %v", err)
	}
}

func TestRegister_DuplicateEmailReturnsConflict(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if err := svc.Register(context.Background(), "Ada", "dup@example.com", "Str0ng!Pass", "user"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := svc.Register(context.Background(), "Ada2", "dup@example.com", "Str0ng!Pass2", "user"); err != ErrEmailAlreadyExists {
		t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
	}
}

func TestLogin_SuccessReturnsTokenPair(t *testing.T) {
	repo := newMemUserRepo()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Str0ng!Pass"), bcrypt.DefaultCost)
	repo.users["ada@example.com"] = &models.User{
		ID:            uuid.New(),
		Email:         "ada@example.com",
		Password:      string(hashed),
		EmailVerified: true,
		Role:          "user",
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	pair, err := svc.Login(context.Background(), "ada@example.com", "Str0ng!Pass")
	if err != nil {
		t.Fatalf("expected login success, got %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty token pair")
	}
}

func TestLogin_WrongPasswordRejected(t *testing.T) {
	repo := newMemUserRepo()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Str0ng!Pass"), bcrypt.DefaultCost)
	repo.users["ada@example.com"] = &models.User{
		ID:            uuid.New(),
		Email:         "ada@example.com",
		Password:      string(hashed),
		EmailVerified: true,
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if _, err := svc.Login(context.Background(), "ada@example.com", "Wrong!123"); err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestLogin_UnverifiedEmailBlocked(t *testing.T) {
	repo := newMemUserRepo()
	hashed, _ := bcrypt.GenerateFromPassword([]byte("Str0ng!Pass"), bcrypt.DefaultCost)
	repo.users["new@example.com"] = &models.User{
		ID:            uuid.New(),
		Email:         "new@example.com",
		Password:      string(hashed),
		EmailVerified: false,
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if _, err := svc.Login(context.Background(), "new@example.com", "Str0ng!Pass"); err == nil {
		t.Fatal("expected unverified email to be blocked")
	}
}

func TestLogin_UnknownEmailRejected(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if _, err := svc.Login(context.Background(), "ghost@example.com", "Whatever1!"); err == nil {
		t.Fatal("expected error for unknown email")
	}
}
