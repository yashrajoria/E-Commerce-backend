package services

import (
	"context"
	"testing"
	"time"

	"auth-service/models"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

type refreshStubTokens struct {
	claims jwt.MapClaims
	role   string
}

func (s refreshStubTokens) GenerateTokenPair(userID, email, role string) (*TokenPair, string, error) {
	s.role = role
	return &TokenPair{AccessToken: "access", RefreshToken: "refresh-new", UserID: userID, Role: role}, "new-jti", nil
}

func (s refreshStubTokens) ValidateToken(string, string) (jwt.MapClaims, error) {
	return s.claims, nil
}

func TestRefreshTokens_UsesRoleFromDB(t *testing.T) {
	uid := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	repo := newMemUserRepo()
	repo.users["u@example.com"] = &models.User{
		ID:    uid,
		Email: "u@example.com",
		Role:  "admin", // promoted in DB after token was issued as user
	}

	tokens := make(map[string]*models.RefreshToken)
	repoGet := &refreshMemRepo{memUserRepo: repo, tokens: tokens}
	jti := "old-jti"
	tokens[jti] = &models.RefreshToken{
		TokenID:   jti,
		UserID:    uid,
		FamilyID:  uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
		Revoked:   false,
	}

	stub := refreshStubTokens{claims: jwt.MapClaims{
		"sub":   uid.String(),
		"jti":   jti,
		"email": "u@example.com",
		"role":  "user", // stale claim
		"typ":   "refresh",
	}}

	svc := &AuthService{userRepo: repoGet, tokenService: stub, db: nil}
	pair, err := svc.RefreshTokens(context.Background(), "any")
	if err != nil {
		t.Fatalf("RefreshTokens: %v", err)
	}
	if pair.Role != "admin" {
		t.Fatalf("expected role from DB admin, got %q", pair.Role)
	}
}

type refreshMemRepo struct {
	*memUserRepo
	tokens map[string]*models.RefreshToken
}

func (r *refreshMemRepo) CreateRefreshToken(_ context.Context, rt *models.RefreshToken) error {
	copy := *rt
	r.tokens[rt.TokenID] = &copy
	return nil
}

func (r *refreshMemRepo) GetRefreshTokenByTokenID(_ context.Context, tokenID string) (*models.RefreshToken, error) {
	rt, ok := r.tokens[tokenID]
	if !ok {
		return nil, errNotFound
	}
	copy := *rt
	return &copy, nil
}

func (r *refreshMemRepo) RevokeRefreshTokenByTokenID(_ context.Context, tokenID string) error {
	if rt, ok := r.tokens[tokenID]; ok {
		rt.Revoked = true
	}
	return nil
}

func (r *refreshMemRepo) RevokeRefreshTokenFamily(_ context.Context, familyID uuid.UUID) error {
	for _, rt := range r.tokens {
		if rt.FamilyID == familyID {
			rt.Revoked = true
		}
	}
	return nil
}

func TestRefreshTokens_ReuseOfRevokedTokenRevokesFamily(t *testing.T) {
	uid := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	repo := newMemUserRepo()
	repo.users["u2@example.com"] = &models.User{ID: uid, Email: "u2@example.com", Role: "user"}

	tokens := make(map[string]*models.RefreshToken)
	repoGet := &refreshMemRepo{memUserRepo: repo, tokens: tokens}

	family := uuid.New()
	jti := "rotated-out-jti"
	tokens[jti] = &models.RefreshToken{
		TokenID:   jti,
		UserID:    uid,
		FamilyID:  family,
		ExpiresAt: time.Now().Add(time.Hour),
		Revoked:   true, // already rotated once; presenting it again is reuse
	}
	sibling := "still-valid-sibling-jti"
	tokens[sibling] = &models.RefreshToken{
		TokenID:   sibling,
		UserID:    uid,
		FamilyID:  family,
		ExpiresAt: time.Now().Add(time.Hour),
		Revoked:   false,
	}

	stub := refreshStubTokens{claims: jwt.MapClaims{
		"sub":   uid.String(),
		"jti":   jti,
		"email": "u2@example.com",
		"role":  "user",
		"typ":   "refresh",
	}}

	svc := &AuthService{userRepo: repoGet, tokenService: stub, db: nil}
	_, err := svc.RefreshTokens(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error on reuse of revoked refresh token")
	}
	if !tokens[sibling].Revoked {
		t.Fatal("expected sibling token in the same family to be revoked after reuse detection")
	}
}

var errNotFound = errRecordNotFound{}

type errRecordNotFound struct{}

func (errRecordNotFound) Error() string { return "record not found" }
