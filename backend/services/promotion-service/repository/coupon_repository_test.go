package repository_test

import (
	"context"
	"promotion-service/models"
	"promotion-service/repository"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// mockCouponRepository is an in-memory implementation of CouponRepository for testing.
type mockCouponRepository struct {
	coupons map[string]*models.Coupon
}

func newMockCouponRepository() repository.CouponRepository {
	return &mockCouponRepository{coupons: make(map[string]*models.Coupon)}
}

func (m *mockCouponRepository) Create(_ context.Context, coupon *models.Coupon) error {
	if coupon.ID == uuid.Nil {
		coupon.ID = uuid.New()
	}
	m.coupons[coupon.Code] = coupon
	return nil
}

func (m *mockCouponRepository) FindByCode(_ context.Context, code string) (*models.Coupon, error) {
	c, ok := m.coupons[code]
	if !ok || !c.Active {
		return nil, gormErrRecordNotFound()
	}
	return c, nil
}

func (m *mockCouponRepository) IncrementUsedCount(_ context.Context, code string) error {
	if c, ok := m.coupons[code]; ok {
		c.UsedCount++
	}
	return nil
}

func (m *mockCouponRepository) Deactivate(_ context.Context, code string) error {
	c, ok := m.coupons[code]
	if !ok {
		return gormErrRecordNotFound()
	}
	c.Active = false
	return nil
}

func (m *mockCouponRepository) FindAll(_ context.Context, _, _ int) ([]models.Coupon, int64, error) {
	var result []models.Coupon
	for _, c := range m.coupons {
		result = append(result, *c)
	}
	return result, int64(len(result)), nil
}

// gormErrRecordNotFound returns a plain error matching GORM's not-found message.
func gormErrRecordNotFound() error {
	return &notFoundError{}
}

type notFoundError struct{}

func (e *notFoundError) Error() string { return "record not found" }

// --- Tests ---

func TestRepository_Create(t *testing.T) {
	repo := newMockCouponRepository()

	coupon := &models.Coupon{
		Code:      "SAVE10",
		Type:      models.CouponTypePercentage,
		Value:     10,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Active:    true,
	}

	err := repo.Create(context.Background(), coupon)
	assert.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, coupon.ID, "ID should be assigned after Create")
}

func TestRepository_FindByCode_Found(t *testing.T) {
	repo := newMockCouponRepository()

	want := &models.Coupon{
		ID:        uuid.New(),
		Code:      "FLAT50",
		Type:      models.CouponTypeFlat,
		Value:     50,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Active:    true,
	}
	_ = repo.Create(context.Background(), want)

	got, err := repo.FindByCode(context.Background(), "FLAT50")
	assert.NoError(t, err)
	assert.Equal(t, want.Code, got.Code)
}

func TestRepository_FindByCode_NotFound(t *testing.T) {
	repo := newMockCouponRepository()

	_, err := repo.FindByCode(context.Background(), "NONEXISTENT")
	assert.Error(t, err)
	assert.EqualError(t, err, "record not found")
}

func TestRepository_IncrementUsedCount(t *testing.T) {
	repo := newMockCouponRepository()

	coupon := &models.Coupon{
		ID:        uuid.New(),
		Code:      "TEST20",
		Type:      models.CouponTypePercentage,
		Value:     20,
		UsedCount: 0,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Active:    true,
	}
	_ = repo.Create(context.Background(), coupon)

	err := repo.IncrementUsedCount(context.Background(), "TEST20")
	assert.NoError(t, err)

	got, _ := repo.FindByCode(context.Background(), "TEST20")
	assert.Equal(t, 1, got.UsedCount)
}

func TestRepository_Deactivate(t *testing.T) {
	repo := newMockCouponRepository()

	coupon := &models.Coupon{
		ID:        uuid.New(),
		Code:      "DEACT",
		Type:      models.CouponTypeFlat,
		Value:     5,
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Active:    true,
	}
	_ = repo.Create(context.Background(), coupon)

	err := repo.Deactivate(context.Background(), "DEACT")
	assert.NoError(t, err)

	_, err = repo.FindByCode(context.Background(), "DEACT")
	assert.Error(t, err, "Deactivated coupon should not be findable")
}
