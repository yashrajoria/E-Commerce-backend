package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CouponType represents the type of discount a coupon provides.
type CouponType string

const (
	CouponTypePercentage   CouponType = "percentage"
	CouponTypeFlat         CouponType = "flat"
	CouponTypeFreeShipping CouponType = "freeshipping"
)

// Coupon represents a promotional coupon stored in Postgres.
type Coupon struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Code          string         `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"`
	Type          CouponType     `gorm:"type:varchar(20);not null" json:"type"`
	Value         float64        `gorm:"not null" json:"value"`                       // discount amount or percentage
	MinOrderValue float64        `gorm:"not null;default:0" json:"min_order_value"`   // minimum cart total to apply
	UsageLimit    int            `gorm:"not null;default:0" json:"usage_limit"`        // 0 = unlimited
	UsedCount     int            `gorm:"not null;default:0" json:"used_count"`
	ExpiresAt     time.Time      `gorm:"not null" json:"expires_at"`
	Active        bool           `gorm:"not null;default:true" json:"active"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// CreateCouponRequest is the payload for creating a new coupon.
type CreateCouponRequest struct {
	Code          string     `json:"code" binding:"required,min=3,max=64"`
	Type          CouponType `json:"type" binding:"required,oneof=percentage flat freeshipping"`
	Value         float64    `json:"value" binding:"required,gte=0"`
	MinOrderValue float64    `json:"min_order_value" binding:"gte=0"`
	UsageLimit    int        `json:"usage_limit" binding:"gte=0"`
	ExpiresAt     time.Time  `json:"expires_at" binding:"required"`
}

// ValidateCouponRequest is the payload for validating a coupon against a cart.
type ValidateCouponRequest struct {
	Code      string  `json:"code" binding:"required"`
	CartTotal float64 `json:"cart_total" binding:"required,gt=0"`
}

// ValidateCouponResponse is the response after validating a coupon.
type ValidateCouponResponse struct {
	Valid          bool       `json:"valid"`
	Code           string     `json:"code"`
	Type           CouponType `json:"type"`
	DiscountAmount float64    `json:"discount_amount"`
	Message        string     `json:"message,omitempty"`
}

// CouponAppliedEvent is published to SNS when a coupon is successfully applied.
type CouponAppliedEvent struct {
	EventType      string    `json:"event_type"`
	CouponID       string    `json:"coupon_id"`
	CouponCode     string    `json:"coupon_code"`
	CouponType     string    `json:"coupon_type"`
	DiscountAmount float64   `json:"discount_amount"`
	CartTotal      float64   `json:"cart_total"`
	Timestamp      time.Time `json:"timestamp"`
}
