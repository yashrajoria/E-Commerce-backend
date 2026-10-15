package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Payment represents a customer payment transaction record within the order and checkout lifecycle.
// It tracks monetary charge amounts, idempotency keys, Stripe gateway interactions, webhook audit logs,
// and refund balances.
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the primary financial settlement record linking customer orders with payment gateways (e.g., Stripe).
//   - Enforces idempotent operations across network retries using unique IdempotencyKey and EventID.
//   - Manages state machine transitions: 'pending' -> 'succeeded' or 'failed', and terminal 'refunded' / 'partially_refunded'.
//   - Records partial and full reimbursement accounting through the RefundedAmount property.
//   - Stores raw webhook event payloads in StripeEventPayload for compliance auditing and dispute investigations.
//   - Scopes multi-tenant store billing attribution via StoreID.
//   - Supports soft deletion through GORM DeletedAt to preserve historical financial records.
//
// Exceptions:
//   - Unique constraint violation if EventID is re-inserted for an existing Stripe webhook event.
//   - Unique constraint violation if IdempotencyKey is duplicated across distinct payment attempts.
//   - Unique constraint violation if StripePaymentID collides with another payment record.
//   - Amount must be a positive integer in smallest currency units (cents or paise).
//   - RefundedAmount cannot exceed Amount and must be non-negative.
//   - Foreign key constraint error if OrderID or UserID does not reference an existing entity.
//   - Invalid status transition error if attempting to modify an already settled terminal state.
//
// Returns / Relations:
//   - Database Table: "payments"
//   - Belongs To: Order via foreign key "OrderID".
//   - Belongs To: User via foreign key "UserID".
//   - Belongs To: Store via optional foreign key "StoreID".
//   - GORM Hooks: Auto-populates CreatedAt, UpdatedAt, and filters out DeletedAt soft deletes.
//
// Parameters / Properties:
//   - Payment_ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - EventID (*string): Webhook event identifier from Stripe for duplicate callback detection (unique index, max 128 chars).
//   - IdempotencyKey (*string): Unique request token preventing duplicate payment captures upon retries (unique index, max 128 chars).
//   - CorrelationID (string): Distributed tracing identifier connecting frontend, order worker, and payment logs (indexed, max 128 chars).
//   - OrderID (uuid.UUID): Identifier of the parent order associated with this payment (indexed, not null).
//   - UserID (uuid.UUID): Identifier of customer submitting payment (indexed, not null).
//   - StoreID (*uuid.UUID): Optional tenant/store identifier for multi-vendor revenue splitting (indexed).
//   - Amount (int): Total charged amount in smallest currency subunits (cents/paise, not null).
//   - RefundedAmount (int64): Cumulative reimbursed amount in currency subunits (defaults to 0, not null).
//   - Currency (string): ISO 4217 three-letter currency code (e.g., "usd", "inr", max 10 chars, not null).
//   - Status (string): Transaction lifecycle status ('pending', 'succeeded', 'failed', 'refunded', max 20 chars, not null).
//   - CheckoutURL (*string): Hosted gateway checkout session URL for customer redirection (max 1024 chars).
//   - StripePaymentID (*string): External Stripe PaymentIntent or Checkout Session ID (unique index).
//   - StripeEventPayload (*string): JSONB serialized raw webhook event data for auditing and troubleshooting.
//   - SucceededAt (*time.Time): Nullable timestamp recorded when gateway confirms charge capture.
//   - FailedAt (*time.Time): Nullable timestamp recorded when payment fails or is declined.
//   - CreatedAt (time.Time): Record creation timestamp, auto-populated by GORM.
//   - UpdatedAt (time.Time): Record modification timestamp, auto-updated by GORM.
//   - DeletedAt (gorm.DeletedAt): Soft-delete timestamp for accounting and audit compliance.
//
// Example:
//
//	idempotencyKey := "idem-pay-87654321"
//	correlationID := "corr-checkout-12345"
//	payment := models.Payment{
//	    IdempotencyKey: &idempotencyKey,
//	    CorrelationID:  correlationID,
//	    OrderID:        uuid.MustParse("a0000000-0000-0000-0000-000000000001"),
//	    UserID:         uuid.MustParse("b0000000-0000-0000-0000-000000000002"),
//	    Amount:         4500, // $45.00
//	    Currency:       "usd",
//	    Status:         "pending",
//	}
type Payment struct {
	Payment_ID         uuid.UUID  `gorm:"type:uuid;json default:gen_random_uuid();primaryKey"`
	EventID            *string    `gorm:"type:varchar(128);uniqueIndex"`
	IdempotencyKey     *string    `gorm:"type:varchar(128);uniqueIndex"` // Ensures idempotent operations for retries
	CorrelationID      string     `gorm:"type:varchar(128);index"`
	OrderID            uuid.UUID  `gorm:"type:uuid;index;not null"`
	UserID             uuid.UUID  `gorm:"type:uuid;index;not null"`
	StoreID            *uuid.UUID `gorm:"type:uuid;index"`
	Amount             int        `gorm:"not null"` // in cents/paise
	RefundedAmount     int64      `gorm:"not null;default:0"`
	Currency           string     `gorm:"type:varchar(10);not null"`
	Status             string     `gorm:"type:varchar(20);not null"`
	CheckoutURL        *string    `gorm:"type:varchar(1024)"` // Nullable URL
	StripePaymentID    *string    `gorm:"uniqueIndex"`
	StripeEventPayload *string    `gorm:"type:jsonb"` // Optional: for audit and debugging
	SucceededAt        *time.Time
	FailedAt           *time.Time
	CreatedAt          time.Time      `gorm:"autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"autoUpdateTime"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

// PaymentMethod represents a vaulted customer payment instrument or token stored for recurring checkout.
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
//	pm := models.PaymentMethod{
//	    UserID:                uuid.MustParse("b0000000-0000-0000-0000-000000000002"),
//	    StripeCustomerID:      "cus_Q1234567890abc",
//	    StripePaymentMethodID: "pm_1N4abcdEFGHIJKLMNOP",
//	    Brand:                 "visa",
//	    Last4:                 "4242",
//	    ExpMonth:              12,
//	    ExpYear:               2028,
//	    IsDefault:             true,
//	}
type PaymentMethod struct {
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
func (PaymentMethod) TableName() string {
	return "user_payment_methods"
}

// PaymentRefund represents a refund transaction issued against a previously settled payment.
// It records full or partial reimbursement amounts, gateway identifiers, audit reasons, and idempotency keys.
//
// BERP Documentation:
//
// Behavior:
//   - Captures reimbursement of funds to customer upon order cancellation, returns, or support adjustments.
//   - Manages refund processing states: 'pending' -> 'succeeded' or 'failed'.
//   - Prevents duplicate reversals across retries by enforcing uniqueness on IdempotencyKey.
//   - Supplies audit justification linking customer requests to accounting adjustments.
//
// Exceptions:
//   - Unique constraint violation if IdempotencyKey is repeated across distinct refund requests.
//   - Amount must be positive and cannot exceed the remaining un-refunded balance on the payment.
//   - Foreign key constraint error if PaymentID does not match an existing payment record.
//   - Refund operation rejected if parent payment status is not 'succeeded' or 'partially_refunded'.
//
// Returns / Relations:
//   - Database Table: "payment_refunds"
//   - Belongs To: Payment via foreign key "PaymentID".
//   - JSON Representation: Serialized with snake_case keys (e.g. "refund_id", "idempotency_key").
//
// Parameters / Properties:
//   - RefundID (string): Unique identifier for the refund, usually assigned by Stripe (e.g., "re_12345", primary key).
//   - PaymentID (uuid.UUID): Foreign key linking to parent payments.payment_id (indexed, not null).
//   - Amount (int64): Reimbursed monetary amount in lowest currency unit (cents/paise, not null).
//   - Currency (string): ISO 4217 three-letter currency code matching parent payment (max 10 chars, not null).
//   - Reason (string): Business or customer service justification for refund (e.g. "order_cancelled", "return", max 256 chars).
//   - Status (string): Refund processing status ('pending', 'succeeded', 'failed', max 32 chars, defaults to 'succeeded').
//   - IdempotencyKey (string): Unique token preventing duplicate gateway refund executions (unique index, max 128 chars).
//   - CreatedAt (time.Time): Record creation timestamp auto-populated by GORM.
//   - UpdatedAt (time.Time): Record modification timestamp auto-updated by GORM.
//
// Example:
//
//	orderID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
//	paymentID := uuid.MustParse("d0000000-0000-0000-0000-000000000004")
//	refund := models.PaymentRefund{
//	    RefundID:       "re_3N4abcdEFGHIJKLM",
//	    PaymentID:      paymentID,
//	    Amount:         2500, // $25.00
//	    Currency:       "usd",
//	    Reason:         "customer_cancellation",
//	    Status:         "succeeded",
//	    IdempotencyKey: "refund-order-" + orderID.String(),
//	}
type PaymentRefund struct {
	RefundID       string    `gorm:"type:varchar(128);primaryKey" json:"refund_id"`
	PaymentID      uuid.UUID `gorm:"type:uuid;not null;index" json:"payment_id"`
	Amount         int64     `gorm:"not null" json:"amount"`
	Currency       string    `gorm:"type:varchar(10);not null" json:"currency"`
	Reason         string    `gorm:"type:varchar(256)" json:"reason"`
	Status         string    `gorm:"type:varchar(32);not null;default:'succeeded'" json:"status"`
	IdempotencyKey string    `gorm:"type:varchar(128);uniqueIndex" json:"idempotency_key"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// StripeProcessedEvent stores Stripe webhook event IDs for exactly-once processing.
type StripeProcessedEvent struct {
	EventID     string    `gorm:"column:event_id;primaryKey"`
	EventType   string    `gorm:"column:event_type;not null"`
	ProcessedAt time.Time `gorm:"column:processed_at;autoCreateTime"`
}

// TableName specifies the custom database table name in GORM.
func (StripeProcessedEvent) TableName() string {
	return "stripe_processed_events"
}
