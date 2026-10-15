package models

import (
	"time"

	"github.com/google/uuid"
)

// UserPaymentMethod represents a vaulted customer payment instrument or token stored for checkout.
// It stores non-sensitive card metadata, customer references, and tokenized provider IDs while adhering
// to PCI DSS compliance.
//
// BERP Documentation:
//
// Behavior:
//   - Stores tokenized payment credentials vaulted securely within payment providers (e.g., Stripe PaymentMethod).
//   - Enables one-click checkout for returning users without storing sensitive PAN or CVV data.
//   - Designates primary payment instruments per customer account via IsDefault flag.
//   - Provides display attributes (Brand, Last4, ExpMonth, ExpYear) for frontend checkout interfaces.
//
// Exceptions:
//   - Unique constraint violation if StripePaymentMethodID is duplicated.
//   - Foreign key constraint error if UserID does not reference an existing User.
//   - Validation error if ExpMonth is outside the range 1-12.
//   - Validation error if ExpYear is in the past.
//   - Validation error if Last4 does not contain exactly 4 digits.
//
// Returns / Relations:
//   - Database Table: "user_payment_methods"
//   - Foreign Key: UserID references users.id.
//   - JSON Representation: Serialized with snake_case keys (e.g. "stripe_payment_method_id", "is_default").
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - UserID (uuid.UUID): Identifier of customer owning this payment instrument (indexed, not null).
//   - StripeCustomerID (string): External Stripe Customer object identifier starting with 'cus_' (not null).
//   - StripePaymentMethodID (string): External Stripe PaymentMethod token identifier starting with 'pm_' (unique index, not null).
//   - Brand (string): Card brand network (e.g., "visa", "mastercard", "amex", max 20 chars, not null).
//   - Last4 (string): Final four digits of the card number (max 4 chars, not null).
//   - ExpMonth (int): Two-digit expiration month: 1 through 12 (not null).
//   - ExpYear (int): Four-digit expiration year, e.g., 2028 (not null).
//   - IsDefault (bool): Indicates if this is the customer's primary payment method (defaults to false).
//   - CreatedAt (time.Time): Creation timestamp auto-populated by GORM.
//   - UpdatedAt (time.Time): Modification timestamp auto-updated by GORM.
//
// Example:
//
//	pm := models.UserPaymentMethod{
//	    UserID:                uuid.MustParse("b0000000-0000-0000-0000-000000000002"),
//	    StripeCustomerID:      "cus_Q1234567890abc",
//	    StripePaymentMethodID: "pm_1N4abcdEFGHIJKLMNOP",
//	    Brand:                 "visa",
//	    Last4:                 "4242",
//	    ExpMonth:              12,
//	    ExpYear:               2028,
//	    IsDefault:             true,
//	}
type UserPaymentMethod struct {
	ID                    uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID                uuid.UUID `gorm:"type:uuid;index;not null" json:"user_id"`
	StripeCustomerID      string    `gorm:"type:text;not null" json:"stripe_customer_id"`
	StripePaymentMethodID string    `gorm:"type:text;uniqueIndex;not null" json:"stripe_payment_method_id"`
	Brand                 string    `gorm:"type:varchar(20);not null" json:"brand"`
	Last4                 string    `gorm:"type:varchar(4);not null" json:"last4"`
	ExpMonth              int       `gorm:"not null" json:"exp_month"`
	ExpYear               int       `gorm:"not null" json:"exp_year"`
	IsDefault             bool      `gorm:"default:false" json:"is_default"`
	CreatedAt             time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt             time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the custom database table name in GORM.
func (UserPaymentMethod) TableName() string {
	return "user_payment_methods"
}
