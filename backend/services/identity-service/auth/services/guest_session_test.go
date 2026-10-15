package services

import (
	"context"
	"errors"
	"testing"

	"identity-service/models"

	"github.com/google/uuid"
)

func TestGuestSession_NewUserSuccess(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	pair, err := svc.GuestSession(context.Background(), GuestSessionInput{
		Email: "guest@example.com",
		Name:  "Guest User",
		Phone: "+447700900000",
	})
	if err != nil {
		t.Fatalf("expected guest session success, got %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected non-empty token pair")
	}

	u, err := repo.FindByEmail(context.Background(), "guest@example.com")
	if err != nil {
		t.Fatalf("expected user persisted, got %v", err)
	}
	if !u.EmailVerified {
		t.Fatal("guest account should have EmailVerified=true")
	}
	if u.VerificationCode != guestAccountMarker {
		t.Fatalf("expected verification code %q, got %q", guestAccountMarker, u.VerificationCode)
	}
	if u.Role != "user" {
		t.Fatalf("expected role 'user', got %q", u.Role)
	}
}

func TestGuestSession_ExistingGuestReused(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	// First guest checkout
	_, err := svc.GuestSession(context.Background(), GuestSessionInput{
		Email: "returning_guest@example.com",
		Name:  "Returning Guest",
	})
	if err != nil {
		t.Fatalf("first guest session failed: %v", err)
	}

	// Second guest checkout with same email
	pair2, err := svc.GuestSession(context.Background(), GuestSessionInput{
		Email: "returning_guest@example.com",
		Name:  "Returning Guest",
	})
	if err != nil {
		t.Fatalf("second guest session should succeed, got %v", err)
	}
	if pair2.AccessToken == "" {
		t.Fatal("expected non-empty token pair for returning guest")
	}
}

func TestGuestSession_RegisteredUserBlockedWithErrAccountExists(t *testing.T) {
	repo := newMemUserRepo()
	// Simulate registered account with user-chosen password and verified email (VerificationCode is empty or a hash, not GUEST_ACCOUNT)
	repo.users["registered@example.com"] = &models.User{
		ID:               uuid.New(),
		Email:            "registered@example.com",
		Password:         "hashedpassword",
		EmailVerified:    true,
		Role:             "user",
		VerificationCode: "", // not a guest marker
	}
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	_, err := svc.GuestSession(context.Background(), GuestSessionInput{
		Email: "registered@example.com",
		Name:  "Someone Else",
	})
	if !errors.Is(err, ErrAccountExists) {
		t.Fatalf("expected ErrAccountExists, got %v", err)
	}
}

func TestGuestSession_EmptyEmailFails(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}

	_, err := svc.GuestSession(context.Background(), GuestSessionInput{
		Email: "   ",
	})
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}
