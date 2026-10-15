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

// Coupon represents a promotional discount configuration applied during cart checkout.
// It tracks discount calculation rules, minimum spend thresholds, usage quotas, expiration schedules,
// multi-tenant store restrictions, and redemption tallies.
//
// BERP Documentation:
//
// Behavior:
//   - Governs promotional pricing strategies across customer checkouts (percentage, flat amount, free shipping).
//   - Validates cart eligibility against MinOrderValue before computing promotional deductions.
//   - Caps total platform liability through UsageLimit and increments UsedCount on completed orders.
//   - Restricts promotional scope to individual merchant tenants when StoreID is provided.
//   - Enables immediate suspension via Active toggle without permanent record deletion.
//   - Retains audit history and financial lineage via soft deletion through GORM DeletedAt.
//
// Exceptions:
//   - Unique constraint violation if Code collides with an existing coupon record.
//   - Validation error if Value is negative, or if Type is percentage and Value exceeds 100.
//   - Validation error if Type is not one of 'percentage', 'flat', or 'freeshipping'.
//   - Redemption rejected if current time is equal to or after ExpiresAt.
//   - Redemption rejected if Active is false or record is soft-deleted.
//   - Redemption rejected if order gross total is strictly below MinOrderValue.
//   - Redemption rejected if UsageLimit is greater than 0 and UsedCount is greater than or equal to UsageLimit.
//
// Returns / Relations:
//   - Database Table: "coupons"
//   - Has Many: CouponUsages ([]CouponUsage) recording redemptions linked by foreign key "CouponID".
//   - Belongs To: Store via optional foreign key "StoreID".
//   - JSON Representation: Serialized with snake_case keys (e.g., "min_order_value", "usage_limit", "expires_at").
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - Code (string): Unique promotional code entered by customers at checkout (unique index, max 64 chars, not null).
//   - Type (CouponType): Discount calculation method: "percentage", "flat", or "freeshipping" (max 20 chars, not null).
//   - Value (float64): Numerical discount value (percentage rate 0-100 or flat deduction in currency units, not null).
//   - MinOrderValue (float64): Minimum order subtotal required to qualify for discount (defaults to 0, not null).
//   - UsageLimit (int): Global redemption limit across all shoppers (0 indicates unlimited redemptions, defaults to 0).
//   - UsedCount (int): Counter tracking total redemptions applied to completed orders (defaults to 0, not null).
//   - ExpiresAt (time.Time): Absolute timestamp after which the coupon is no longer valid (not null).
//   - Active (bool): Operational switch to enable or pause the coupon (defaults to true, not null).
//   - StoreID (*uuid.UUID): Optional tenant store ID restricting promotion to a specific vendor store.
//   - CreatedAt (time.Time): Timestamp when coupon was created, auto-populated by GORM.
//   - UpdatedAt (time.Time): Timestamp when coupon was last modified, auto-updated by GORM.
//   - DeletedAt (gorm.DeletedAt): Soft-delete timestamp preserving historical order discounts during audit.
//
// Example:
//
//	expires := time.Now().Add(30 * 24 * time.Hour)
//	coupon := models.Coupon{
//	    Code:          "SAVE20",
//	    Type:          models.CouponTypePercentage,
//	    Value:         20.0, // 20% discount
//	    MinOrderValue: 50.00,
//	    UsageLimit:    500,
//	    ExpiresAt:     expires,
//	    Active:        true,
//	}
type Coupon struct {
	ID            uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Code          string         `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"`
	Type          CouponType     `gorm:"type:varchar(20);not null" json:"type"`
	Value         float64        `gorm:"not null" json:"value"`                     // discount amount or percentage
	MinOrderValue float64        `gorm:"not null;default:0" json:"min_order_value"` // minimum cart total to apply
	UsageLimit    int            `gorm:"not null;default:0" json:"usage_limit"`     // 0 = unlimited
	UsedCount     int            `gorm:"not null;default:0" json:"used_count"`
	ExpiresAt     time.Time      `gorm:"not null" json:"expires_at"`
	Active        bool           `gorm:"not null;default:true" json:"active"`
	StoreID       *uuid.UUID     `gorm:"type:uuid;index" json:"store_id,omitempty"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// CouponUsage represents a single redemption record of a promotional coupon against an order.
// It establishes a historical link between the customer, the redeemed coupon, and the resulting purchase,
// guaranteeing that coupon benefits cannot be applied more than once per order.
//
// BERP Documentation:
//
// Behavior:
//   - Records completed redemption events when an order is finalized with an active coupon.
//   - Enforces single-coupon-per-order rule via unique index constraint on OrderID.
//   - Supports fraud prevention and customer redemption analytics via composite index on (CouponID, UserID).
//   - Preserves immutable historical evidence of discount utilization for financial auditing.
//
// Exceptions:
//   - Unique constraint violation (idx_coupon_usages_order_id) if multiple coupons are applied to the same order.
//   - Foreign key constraint error if CouponID does not reference an existing coupon record.
//   - Foreign key constraint error if OrderID does not reference an existing order record.
//   - Foreign key constraint error if UserID does not reference an existing customer record.
//
// Returns / Relations:
//   - Database Table: "coupon_usages"
//   - Belongs To: Coupon via foreign key "CouponID" (indexed in composite idx_coupon_usages_coupon_user).
//   - Belongs To: Order via foreign key "OrderID" (unique index idx_coupon_usages_order_id).
//   - Belongs To: User via foreign key "UserID" (indexed in composite idx_coupon_usages_coupon_user).
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - CouponID (uuid.UUID): Foreign key linking to parent coupons.id (not null, indexed).
//   - OrderID (uuid.UUID): Foreign key linking to parent orders.id (not null, unique index).
//   - UserID (uuid.UUID): Foreign key linking to customer users.id (not null, indexed).
//   - CreatedAt (time.Time): Timestamp when coupon redemption was persisted, auto-populated by GORM.
//
// Example:
//
//	usage := models.CouponUsage{
//	    CouponID: coupon.ID,
//	    OrderID:  order.ID,
//	    UserID:   order.UserID,
//	}
type CouponUsage struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CouponID  uuid.UUID `gorm:"type:uuid;not null;index:idx_coupon_usages_coupon_user"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_coupon_usages_order_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index:idx_coupon_usages_coupon_user"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
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
