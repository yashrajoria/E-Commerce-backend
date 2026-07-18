package services

import (
	"context"
	"errors"
	"testing"

	"auth-service/models"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type memUserRepo struct {
	users map[string]*models.User
}

func newMemUserRepo() *memUserRepo {
	return &memUserRepo{users: make(map[string]*models.User)}
}

func (r *memUserRepo) FindByEmail(_ context.Context, email string) (*models.User, error) {
	u, ok := r.users[email]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *u
	return &copy, nil
}
func (r *memUserRepo) FindByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	for _, u := range r.users {
		if u.ID == id {
			copy := *u
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (r *memUserRepo) Create(_ context.Context, user *models.User) error {
	if _, exists := r.users[user.Email]; exists {
		return errors.New("duplicate")
	}
	copy := *user
	r.users[user.Email] = &copy
	return nil
}
func (r *memUserRepo) Update(_ context.Context, user *models.User) error {
	r.users[user.Email] = user
	return nil
}
func (r *memUserRepo) CreateRefreshToken(context.Context, *models.RefreshToken) error {
	return nil
}
func (r *memUserRepo) GetRefreshTokenByTokenID(context.Context, string) (*models.RefreshToken, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *memUserRepo) RevokeRefreshTokenByTokenID(context.Context, string) error { return nil }
func (r *memUserRepo) CountByRole(_ context.Context, role string) (int64, error) {
	var n int64
	for _, u := range r.users {
		if u.Role == role {
			n++
		}
	}
	return n, nil
}

type stubTokenService struct{}

func (stubTokenService) GenerateTokenPair(userID, email, role string) (*TokenPair, string, error) {
	return &TokenPair{AccessToken: "a", RefreshToken: "r", Role: role}, "tid", nil
}
func (stubTokenService) ValidateToken(string, string) (jwt.MapClaims, error) {
	return jwt.MapClaims{}, nil
}

func TestBootstrapAdminFromEnv_SkippedWithoutEnv(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}
	t.Setenv("ADMIN_EMAIL", "")
	t.Setenv("ADMIN_PASSWORD", "")
	if err := svc.BootstrapAdminFromEnv(context.Background()); err != nil {
		t.Fatalf("expected skip, got %v", err)
	}
}

func TestBootstrapAdminFromEnv_SkippedWhenAdminExists(t *testing.T) {
	repo := newMemUserRepo()
	_ = repo.Create(context.Background(), &models.User{
		ID: uuid.New(), Email: "existing@example.com", Role: "admin", Password: "x", Name: "A",
	})
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}
	t.Setenv("ADMIN_EMAIL", "new@example.com")
	t.Setenv("ADMIN_PASSWORD", "Bootstrap1!")
	if err := svc.BootstrapAdminFromEnv(context.Background()); err != nil {
		t.Fatalf("expected skip, got %v", err)
	}
	if len(repo.users) != 1 {
		t.Fatalf("expected no new user, got %d", len(repo.users))
	}
}

func TestBootstrapAdminFromEnv_RejectsWeakPasswordWhenNoAdmins(t *testing.T) {
	repo := newMemUserRepo()
	svc := &AuthService{userRepo: repo, tokenService: stubTokenService{}, db: nil}
	t.Setenv("ADMIN_EMAIL", "weak@example.com")
	t.Setenv("ADMIN_PASSWORD", "weak")
	if err := svc.BootstrapAdminFromEnv(context.Background()); err == nil {
		t.Fatal("expected weak password error")
	}
}
