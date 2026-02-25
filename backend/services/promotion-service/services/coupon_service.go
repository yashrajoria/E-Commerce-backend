package services

import (
	"context"
	"encoding/json"
	"fmt"
	"promotion-service/models"
	"promotion-service/repository"
	"strings"
	"time"

	aws_pkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
	"go.uber.org/zap"
)

// ServiceError represents a typed error with an HTTP status code.
type ServiceError struct {
	StatusCode int
	Message    string
}

func (e *ServiceError) Error() string {
	return e.Message
}

// CouponService defines the interface for coupon business logic.
type CouponService interface {
	CreateCoupon(ctx context.Context, req *models.CreateCouponRequest) (*models.Coupon, *ServiceError)
	ValidateCoupon(ctx context.Context, req *models.ValidateCouponRequest) (*models.ValidateCouponResponse, *ServiceError)
	GetCoupon(ctx context.Context, code string) (*models.Coupon, *ServiceError)
	DeactivateCoupon(ctx context.Context, code string) *ServiceError
	ListCoupons(ctx context.Context, page, limit int) ([]models.Coupon, int64, *ServiceError)
}

// couponServiceImpl implements CouponService.
type couponServiceImpl struct {
	repo        repository.CouponRepository
	snsClient   aws_pkg.SNSPublisher
	snsTopicArn string
	logger      *zap.Logger
}

// NewCouponService creates a new CouponService.
func NewCouponService(
	repo repository.CouponRepository,
	snsClient aws_pkg.SNSPublisher,
	snsTopicArn string,
	logger *zap.Logger,
) CouponService {
	return &couponServiceImpl{
		repo:        repo,
		snsClient:   snsClient,
		snsTopicArn: snsTopicArn,
		logger:      logger,
	}
}

// CreateCoupon creates a new coupon.
func (s *couponServiceImpl) CreateCoupon(ctx context.Context, req *models.CreateCouponRequest) (*models.Coupon, *ServiceError) {
	if req.ExpiresAt.Before(time.Now()) {
		return nil, &ServiceError{StatusCode: 400, Message: "Expiry date must be in the future"}
	}

	if req.Type == models.CouponTypePercentage && req.Value > 100 {
		return nil, &ServiceError{StatusCode: 400, Message: "Percentage discount cannot exceed 100"}
	}

	coupon := &models.Coupon{
		Code:          strings.ToUpper(req.Code),
		Type:          req.Type,
		Value:         req.Value,
		MinOrderValue: req.MinOrderValue,
		UsageLimit:    req.UsageLimit,
		ExpiresAt:     req.ExpiresAt,
		Active:        true,
	}

	if err := s.repo.Create(ctx, coupon); err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, &ServiceError{StatusCode: 409, Message: "Coupon code already exists"}
		}
		s.logger.Error("Failed to create coupon", zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to create coupon"}
	}

	s.logger.Info("Coupon created", zap.String("code", coupon.Code), zap.String("type", string(coupon.Type)))
	return coupon, nil
}

// ValidateCoupon validates a coupon against a cart total and returns the discount amount.
func (s *couponServiceImpl) ValidateCoupon(ctx context.Context, req *models.ValidateCouponRequest) (*models.ValidateCouponResponse, *ServiceError) {
	coupon, err := s.repo.FindByCode(ctx, req.Code)
	if err != nil {
		return &models.ValidateCouponResponse{
			Valid:   false,
			Code:    req.Code,
			Message: "Coupon not found or inactive",
		}, nil
	}

	// Check expiry
	if time.Now().After(coupon.ExpiresAt) {
		return &models.ValidateCouponResponse{
			Valid:   false,
			Code:    req.Code,
			Message: "Coupon has expired",
		}, nil
	}

	// Check usage limit
	if coupon.UsageLimit > 0 && coupon.UsedCount >= coupon.UsageLimit {
		return &models.ValidateCouponResponse{
			Valid:   false,
			Code:    req.Code,
			Message: "Coupon usage limit reached",
		}, nil
	}

	// Check minimum order value
	if req.CartTotal < coupon.MinOrderValue {
		return &models.ValidateCouponResponse{
			Valid:   false,
			Code:    req.Code,
			Message: fmt.Sprintf("Minimum order value of %.2f required", coupon.MinOrderValue),
		}, nil
	}

	// Calculate discount
	var discount float64
	switch coupon.Type {
	case models.CouponTypePercentage:
		discount = req.CartTotal * (coupon.Value / 100)
	case models.CouponTypeFlat:
		discount = coupon.Value
		if discount > req.CartTotal {
			discount = req.CartTotal
		}
	case models.CouponTypeFreeShipping:
		discount = 0 // handled by shipping service; flag only
	default:
		return nil, &ServiceError{StatusCode: 500, Message: "Unknown coupon type"}
	}

	// Increment usage
	if err := s.repo.IncrementUsedCount(ctx, req.Code); err != nil {
		s.logger.Error("Failed to increment coupon usage", zap.String("code", req.Code), zap.Error(err))
		return nil, &ServiceError{StatusCode: 500, Message: "Failed to apply coupon"}
	}

	// Publish coupon_applied event
	s.publishCouponAppliedEvent(ctx, coupon, discount, req.CartTotal)

	return &models.ValidateCouponResponse{
		Valid:          true,
		Code:           coupon.Code,
		Type:           coupon.Type,
		DiscountAmount: discount,
		Message:        "Coupon applied successfully",
	}, nil
}

// GetCoupon retrieves a coupon by code.
func (s *couponServiceImpl) GetCoupon(ctx context.Context, code string) (*models.Coupon, *ServiceError) {
	coupon, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, &ServiceError{StatusCode: 404, Message: "Coupon not found"}
	}
	return coupon, nil
}

// DeactivateCoupon deactivates a coupon by code.
func (s *couponServiceImpl) DeactivateCoupon(ctx context.Context, code string) *ServiceError {
	if err := s.repo.Deactivate(ctx, code); err != nil {
		if err.Error() == "record not found" {
			return &ServiceError{StatusCode: 404, Message: "Coupon not found"}
		}
		s.logger.Error("Failed to deactivate coupon", zap.String("code", code), zap.Error(err))
		return &ServiceError{StatusCode: 500, Message: "Failed to deactivate coupon"}
	}

	s.logger.Info("Coupon deactivated", zap.String("code", code))
	return nil
}

// ListCoupons returns paginated coupons.
func (s *couponServiceImpl) ListCoupons(ctx context.Context, page, limit int) ([]models.Coupon, int64, *ServiceError) {
	coupons, total, err := s.repo.FindAll(ctx, page, limit)
	if err != nil {
		s.logger.Error("Failed to list coupons", zap.Error(err))
		return nil, 0, &ServiceError{StatusCode: 500, Message: "Failed to list coupons"}
	}
	return coupons, total, nil
}

// publishCouponAppliedEvent publishes a coupon_applied event to SNS.
func (s *couponServiceImpl) publishCouponAppliedEvent(ctx context.Context, coupon *models.Coupon, discount, cartTotal float64) {
	if s.snsClient == nil || s.snsTopicArn == "" {
		s.logger.Warn("SNS client not configured, skipping coupon_applied event")
		return
	}

	event := models.CouponAppliedEvent{
		EventType:      "coupon_applied",
		CouponID:       coupon.ID.String(),
		CouponCode:     coupon.Code,
		CouponType:     string(coupon.Type),
		DiscountAmount: discount,
		CartTotal:      cartTotal,
		Timestamp:      time.Now(),
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("Failed to marshal coupon_applied event", zap.Error(err))
		return
	}

	if err := s.snsClient.Publish(ctx, s.snsTopicArn, eventBytes); err != nil {
		s.logger.Error("Failed to publish coupon_applied event", zap.Error(err))
		return
	}

	s.logger.Info("Published coupon_applied event",
		zap.String("coupon_code", coupon.Code),
		zap.Float64("discount", discount),
	)
}
