package services_test

import (
	"context"
	"promotion-service/models"
	"promotion-service/repository"
	"promotion-service/services"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// --- Mock Repository ---

type mockRepo struct {
	coupons map[string]*models.Coupon
}

func newMockRepo() repository.CouponRepository {
	return &mockRepo{coupons: make(map[string]*models.Coupon)}
}

func (m *mockRepo) Create(_ context.Context, c *models.Coupon) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	m.coupons[c.Code] = c
	return nil
}

func (m *mockRepo) FindByCode(_ context.Context, code string) (*models.Coupon, error) {
	c, ok := m.coupons[code]
	if !ok || !c.Active {
		return nil, &mockNotFoundError{}
	}
	return c, nil
}

func (m *mockRepo) IncrementUsedCount(_ context.Context, code string) error {
	if c, ok := m.coupons[code]; ok {
		c.UsedCount++
	}
	return nil
}

func (m *mockRepo) Deactivate(_ context.Context, code string) error {
	c, ok := m.coupons[code]
	if !ok {
		return &mockNotFoundError{}
	}
	c.Active = false
	return nil
}

func (m *mockRepo) FindAll(_ context.Context, _, _ int) ([]models.Coupon, int64, error) {
	var result []models.Coupon
	for _, c := range m.coupons {
		result = append(result, *c)
	}
	return result, int64(len(result)), nil
}

type mockNotFoundError struct{}

func (e *mockNotFoundError) Error() string { return "record not found" }

// --- Mock SNS Publisher ---

type mockSNSPublisher struct {
	published []string
}

func (m *mockSNSPublisher) Publish(_ context.Context, topicArn string, _ []byte) error {
	m.published = append(m.published, topicArn)
	return nil
}

// --- Helpers ---

func newTestService(repo repository.CouponRepository, sns *mockSNSPublisher) services.CouponService {
	logger, _ := zap.NewDevelopment()
	return services.NewCouponService(repo, sns, "arn:aws:sns:us-east-1:000000000000:promotion-events", logger)
}

func activeCoupon(code string, couponType models.CouponType, value, minOrder float64, usageLimit, usedCount int) *models.Coupon {
	return &models.Coupon{
		ID:            uuid.New(),
		Code:          code,
		Type:          couponType,
		Value:         value,
		MinOrderValue: minOrder,
		UsageLimit:    usageLimit,
		UsedCount:     usedCount,
		ExpiresAt:     time.Now().Add(24 * time.Hour),
		Active:        true,
	}
}

// --- Tests ---

func TestService_CreateCoupon_Success(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	req := &models.CreateCouponRequest{
		Code:       "SAVE10",
		Type:       models.CouponTypePercentage,
		Value:      10,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		UsageLimit: 100,
	}

	coupon, svcErr := svc.CreateCoupon(context.Background(), req)
	assert.Nil(t, svcErr)
	assert.NotNil(t, coupon)
	assert.Equal(t, "SAVE10", coupon.Code) // code is uppercased
}

func TestService_CreateCoupon_ExpiredDate(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	req := &models.CreateCouponRequest{
		Code:      "OLD",
		Type:      models.CouponTypeFlat,
		Value:     5,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // past
	}

	_, svcErr := svc.CreateCoupon(context.Background(), req)
	assert.NotNil(t, svcErr)
	assert.Equal(t, 400, svcErr.StatusCode)
}

func TestService_ValidateCoupon_Percentage(t *testing.T) {
	repo := newMockRepo()
	sns := &mockSNSPublisher{}
	svc := newTestService(repo, sns)

	coupon := activeCoupon("TENOFF", models.CouponTypePercentage, 10, 0, 0, 0)
	_ = repo.Create(context.Background(), coupon)

	resp, svcErr := svc.ValidateCoupon(context.Background(), &models.ValidateCouponRequest{
		Code:      "TENOFF",
		CartTotal: 100,
	})

	assert.Nil(t, svcErr)
	assert.True(t, resp.Valid)
	assert.Equal(t, 10.0, resp.DiscountAmount)
	assert.Len(t, sns.published, 1, "Should publish SNS event")
}

func TestService_ValidateCoupon_Flat(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	coupon := activeCoupon("FLAT20", models.CouponTypeFlat, 20, 0, 0, 0)
	_ = repo.Create(context.Background(), coupon)

	resp, svcErr := svc.ValidateCoupon(context.Background(), &models.ValidateCouponRequest{
		Code:      "FLAT20",
		CartTotal: 50,
	})

	assert.Nil(t, svcErr)
	assert.True(t, resp.Valid)
	assert.Equal(t, 20.0, resp.DiscountAmount)
}

func TestService_ValidateCoupon_FlatCapAtCartTotal(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	coupon := activeCoupon("BIGSAVE", models.CouponTypeFlat, 200, 0, 0, 0)
	_ = repo.Create(context.Background(), coupon)

	resp, svcErr := svc.ValidateCoupon(context.Background(), &models.ValidateCouponRequest{
		Code:      "BIGSAVE",
		CartTotal: 50,
	})

	assert.Nil(t, svcErr)
	assert.True(t, resp.Valid)
	assert.Equal(t, 50.0, resp.DiscountAmount, "Flat discount capped at cart total")
}

func TestService_ValidateCoupon_UsageLimitReached(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	coupon := activeCoupon("LIMITED", models.CouponTypePercentage, 5, 0, 10, 10)
	_ = repo.Create(context.Background(), coupon)

	resp, svcErr := svc.ValidateCoupon(context.Background(), &models.ValidateCouponRequest{
		Code:      "LIMITED",
		CartTotal: 100,
	})

	assert.Nil(t, svcErr)
	assert.False(t, resp.Valid)
	assert.Contains(t, resp.Message, "usage limit")
}

func TestService_ValidateCoupon_MinOrderNotMet(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	coupon := activeCoupon("MINVAL", models.CouponTypePercentage, 10, 100, 0, 0)
	_ = repo.Create(context.Background(), coupon)

	resp, svcErr := svc.ValidateCoupon(context.Background(), &models.ValidateCouponRequest{
		Code:      "MINVAL",
		CartTotal: 50,
	})

	assert.Nil(t, svcErr)
	assert.False(t, resp.Valid)
	assert.Contains(t, resp.Message, "Minimum order")
}

func TestService_ValidateCoupon_Expired(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	coupon := &models.Coupon{
		ID:        uuid.New(),
		Code:      "EXPIRED",
		Type:      models.CouponTypeFlat,
		Value:     10,
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		Active:    true,
	}
	_ = repo.Create(context.Background(), coupon)

	resp, svcErr := svc.ValidateCoupon(context.Background(), &models.ValidateCouponRequest{
		Code:      "EXPIRED",
		CartTotal: 50,
	})

	assert.Nil(t, svcErr)
	assert.False(t, resp.Valid)
	assert.Contains(t, resp.Message, "expired")
}

func TestService_DeactivateCoupon_Success(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	coupon := activeCoupon("TODEACT", models.CouponTypeFlat, 5, 0, 0, 0)
	_ = repo.Create(context.Background(), coupon)

	svcErr := svc.DeactivateCoupon(context.Background(), "TODEACT")
	assert.Nil(t, svcErr)
}

func TestService_DeactivateCoupon_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	svcErr := svc.DeactivateCoupon(context.Background(), "GHOST")
	assert.NotNil(t, svcErr)
	assert.Equal(t, 404, svcErr.StatusCode)
}

func TestService_ListCoupons(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo, &mockSNSPublisher{})

	for _, code := range []string{"C1", "C2", "C3"} {
		_ = repo.Create(context.Background(), activeCoupon(code, models.CouponTypeFlat, 5, 0, 0, 0))
	}

	coupons, total, svcErr := svc.ListCoupons(context.Background(), 1, 10)
	assert.Nil(t, svcErr)
	assert.Equal(t, int64(3), total)
	assert.Len(t, coupons, 3)
}
