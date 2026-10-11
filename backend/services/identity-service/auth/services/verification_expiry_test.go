package services

import (
	"context"
	"testing"
	"time"

	"identity-service/models"

	"github.com/google/uuid"
)

func TestVerifyEmail_ExpiredCodeRejected(t *testing.T) {
	repo := newMemUserRepo()
	past := time.Now().Add(-time.Minute)
	repo.users["e@example.com"] = &models.User{
		ID: uuid.New(), Email: "e@example.com",
		VerificationCode: hashVerificationCode("123456"), VerificationExpiresAt: &past,
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	if err := svc.VerifyEmail(context.Background(), "e@example.com", "123456"); err == nil {
		t.Fatal("expected expired code to be rejected")
	}
	if repo.users["e@example.com"].EmailVerified {
		t.Fatal("expired code must not verify the account")
	}

	future := time.Now().Add(time.Minute)
	repo.users["e@example.com"].VerificationExpiresAt = &future
	if err := svc.VerifyEmail(context.Background(), "e@example.com", "123456"); err != nil {
		t.Fatalf("valid unexpired code should verify: %v", err)
	}
}
