// Package models defines database schema entities, payment allocations, and transactional outbox events.
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Order represents a customer purchase in the order management system.
// It tracks monetary amounts, coupon discounts, lifecycle states, tenant/store attribution,
// and associated line items.
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the aggregate root for all customer checkout transactions.
//   - Tracks lifecycle transitions: 'pending_payment' -> 'paid' -> 'processing' -> 'completed' (or 'canceled').
//   - Manages refund accounting through RefundedAmount and discount deductions via DiscountAmount.
//   - Supports soft deletion through GORM to preserve financial and audit records.
//
// Exceptions:
//   - Unique constraint violation if OrderNumber is duplicated.
//   - Unique constraint violation if IdempotencyKey is duplicated across orders.
//   - Amount must be a positive integer in smallest currency units (cents).
//   - RefundedAmount cannot exceed Amount.
//   - Cannot transition to 'completed' if order is in 'canceled' status.
//   - Foreign key violation if UserID, StoreID, or CouponID do not refer to existing entities.
//
// Returns / Relations:
//   - Database Table: "orders"
//   - Has Many: OrderItems ([]OrderItem) with foreign key "OrderID" and constraint OnDelete: CASCADE.
//   - Foreign Keys: UserID (Users), StoreID (Stores, optional), CouponID (Coupons, optional).
//   - JSON Representation: Serialized as a JSON object with camelCase or snake_case tags (e.g., refunded_amount).
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - OrderNumber (string): Unique human-facing order reference (e.g., "ORD-20261015-ABCD").
//   - IdempotencyKey (*string): Unique request token preventing duplicate orders upon network retries (max 128 chars).
//   - UserID (uuid.UUID): Identifier of customer placing the order; indexed for fast lookup.
//   - Amount (int): Total gross payable amount in currency subunits (e.g., 2500 for $25.00).
//   - RefundedAmount (int64): Cumulative amount refunded in currency subunits (defaults to 0).
//   - CouponCode (string): Applied promotion code string (e.g., "SAVE20", max 50 chars).
//   - CouponID (*uuid.UUID): Optional reference to the applied Coupon entity.
//   - DiscountAmount (int): Amount subtracted from total due to applied coupon, in cents.
//   - Currency (string): ISO 4217 three-letter currency code (defaults to "usd", max 10 chars).
//   - Status (string): Order workflow state ('pending_payment', 'paid', 'canceled', 'completed').
//   - StoreID (*uuid.UUID): Tenant/vendor store identifier for multi-vendor marketplaces.
//   - CanceledAt (*time.Time): Nullable timestamp set when the order is marked as canceled.
//   - CompletedAt (*time.Time): Nullable timestamp set when the order is marked as fulfilled/completed.
//   - CreatedAt (time.Time): Creation timestamp, automatically populated by GORM.
//   - UpdatedAt (time.Time): Last modification timestamp, automatically updated by GORM.
//   - DeletedAt (gorm.DeletedAt): Soft-delete timestamp; records remain in DB for historical auditing.
//   - OrderItems ([]OrderItem): Associated line items purchased in this order.
//
// Example:
//
//	idempotencyKey := "idem-order-98765"
//	storeID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
//	order := models.Order{
//	    OrderNumber:    "ORD-20261015-8941",
//	    IdempotencyKey: &idempotencyKey,
//	    UserID:         uuid.MustParse("b0000000-0000-0000-0000-000000000002"),
//	    Amount:         4500, // $45.00 total
//	    RefundedAmount: 0,
//	    CouponCode:     "FALL10",
//	    DiscountAmount: 500,  // $5.00 off
//	    Currency:       "usd",
//	    Status:         "pending_payment",
//	    StoreID:        &storeID,
//	    OrderItems: []models.OrderItem{
//	        {
//	            ProductID: uuid.MustParse("c0000000-0000-0000-0000-000000000003"),
//	            Quantity:  2,
//	            Price:     2500, // $25.00 each
//	        },
//	    },
//	}
type Order struct {
	ID             uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderNumber    string         `gorm:"uniqueIndex;not null"`
	IdempotencyKey *string        `gorm:"type:varchar(128);uniqueIndex"`
	UserID         uuid.UUID      `gorm:"type:uuid;not null;index"`
	Amount         int            `gorm:"not null"`
	RefundedAmount int64          `gorm:"not null;default:0" json:"refunded_amount"`
	CouponCode     string         `gorm:"type:varchar(50)"`
	CouponID       *uuid.UUID     `gorm:"type:uuid;index"`
	DiscountAmount int            `gorm:"default:0"`
	Currency       string         `gorm:"type:varchar(10);not null;default:'usd'"`
	Status         string         `gorm:"type:varchar(20);not null;default:'pending_payment'"`
	StoreID        *uuid.UUID     `gorm:"type:uuid;index"`
	CanceledAt     *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	OrderItems     []OrderItem    `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE"`
}

// OrderItem represents an individual line item within an order.
// It captures the product ID, unit price at purchase time, and quantity ordered.
//
// BERP Documentation:
//
// Behavior:
//   - Associates a specific product with an order at fixed price snapshot.
//   - Preserves historical purchase price even if the product catalog price changes later.
//   - Cascades deletion when the owning parent Order is permanently removed.
//
// Exceptions:
//   - Quantity must be greater than zero.
//   - Price must be non-negative.
//   - Foreign key constraint error if OrderID does not match an existing Order record.
//   - Foreign key constraint error if ProductID does not refer to a valid Product.
//
// Returns / Relations:
//   - Database Table: "order_items"
//   - Belongs To: Order via foreign key "OrderID".
//   - References: Product via "ProductID".
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - OrderID (uuid.UUID): Foreign key linking to parent orders.id (indexed).
//   - ProductID (uuid.UUID): Identifier of the purchased product catalog item (indexed).
//   - Quantity (int): Number of units purchased (must be >= 1).
//   - Price (int): Unit price in lowest currency unit (cents) at the time of purchase.
//
// Example:
//
//	orderItem := models.OrderItem{
//	    OrderID:   order.ID,
//	    ProductID: uuid.MustParse("c0000000-0000-0000-0000-000000000003"),
//	    Quantity:  2,
//	    Price:     2500, // $25.00 each
//	}
type OrderItem struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;index"`
	ProductID uuid.UUID `gorm:"type:uuid;not null;index"`
	Quantity  int       `gorm:"not null"`
	Price     int       `gorm:"not null"`
}

// Outbox event status constants indicate the dispatch lifecycle stage of an asynchronous message.
const (
	// OutboxStatusPending indicates the event is waiting for a background worker to claim and publish it.
	OutboxStatusPending = "pending"
	// OutboxStatusProcessing indicates a worker has acquired a lease lock and is currently attempting delivery.
	OutboxStatusProcessing = "processing"
	// OutboxStatusPublished indicates the event was confirmed received by the destination queue or topic.
	OutboxStatusPublished = "published"
	// OutboxStatusFailed indicates the event exceeded maximum retry attempts or suffered an unrecoverable failure.
	OutboxStatusFailed = "failed"
)

// Outbox destination constants specify the supported target cloud messaging brokers.
const (
	// OutboxDestinationSQS designates an AWS Simple Queue Service (SQS) target queue.
	OutboxDestinationSQS = "sqs"
	// OutboxDestinationSNS designates an AWS Simple Notification Service (SNS) target topic.
	OutboxDestinationSNS = "sns"
)

// OutboxEvent implements the Transactional Outbox Pattern for the order service.
// Events are written to the database inside the same database transaction as the business entity changes,
// ensuring guaranteed at-least-once message delivery to distributed message brokers (SQS/SNS).
//
// BERP Documentation:
//
// Behavior:
//   - Provides durable asynchronous event publishing decoupled from external broker availability.
//   - Uses distributed leasing (LeaseOwner, LeaseExpiresAt) to prevent duplicate concurrent processing.
//   - Increments Attempts and records LastError when dispatches fail, backing off with AvailableAt.
//   - Marks status as 'published' with PublishedAt upon successful broker acknowledgment.
//
// Exceptions:
//   - Database constraint error if AggregateType, AggregateID, EventType, DestinationType, or Destination is empty.
//   - Serialization error if Payload contains invalid or unparseable JSON bytes.
//   - Lease collision if another worker attempts to claim an event before LeaseExpiresAt has passed.
//   - Max attempts exceeded moves event to OutboxStatusFailed for dead-letter inspection.
//
// Returns / Relations:
//   - Database Table: "outbox_events"
//   - Payload: Stores serialized JSONB message data forwarded to the messaging broker.
//   - Destination: Direct SQS Queue URL or SNS Topic ARN.
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via gen_random_uuid().
//   - AggregateType (string): Name of the domain aggregate root (e.g., "Order", max 100 chars).
//   - AggregateID (uuid.UUID): ID of the aggregate entity that triggered the event.
//   - EventType (string): Dot-delimited event name (e.g., "order.created", "order.paid", max 150 chars).
//   - DestinationType (string): Target messaging protocol: 'sqs' or 'sns' (max 20 chars).
//   - Destination (string): SQS queue URL or SNS topic ARN.
//   - Payload ([]byte): Raw JSONB encoded event payload.
//   - Status (string): Current event dispatch lifecycle status ('pending', 'processing', 'published', 'failed').
//   - Attempts (int): Number of dispatch attempts made so far (defaults to 0).
//   - AvailableAt (time.Time): Scheduled timestamp after which the event is eligible to be claimed.
//   - LeaseOwner (*string): Worker or instance ID holding the processing lock (max 128 chars).
//   - LeaseExpiresAt (*time.Time): Lock expiration timestamp; releases event if worker crashes.
//   - ClaimedAt (*time.Time): Timestamp when worker claimed the current lease.
//   - PublishedAt (*time.Time): Timestamp when broker successfully acknowledged the event.
//   - LastError (*string): Diagnostic error message from the most recent failed dispatch attempt.
//   - CreatedAt (time.Time): Timestamp when event was inserted into the outbox.
//   - UpdatedAt (time.Time): Timestamp when event record was last modified.
//
// Example:
//
//	orderCreatedPayload := []byte(`{"order_id":"` + order.ID.String() + `","amount":4500,"currency":"usd"}`)
//	outboxEvent := models.OutboxEvent{
//	    AggregateType:   "Order",
//	    AggregateID:     order.ID,
//	    EventType:       "order.created",
//	    DestinationType: models.OutboxDestinationSNS,
//	    Destination:     "arn:aws:sns:eu-west-1:123456789012:order-events",
//	    Payload:         orderCreatedPayload,
//	    Status:          models.OutboxStatusPending,
//	    AvailableAt:     time.Now().UTC(),
//	}
type OutboxEvent struct {
	ID              uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	AggregateType   string    `gorm:"type:varchar(100);not null"`
	AggregateID     uuid.UUID `gorm:"type:uuid;not null"`
	EventType       string    `gorm:"type:varchar(150);not null"`
	DestinationType string    `gorm:"type:varchar(20);not null"`
	Destination     string    `gorm:"type:text;not null"`
	Payload         []byte    `gorm:"type:jsonb;not null"`
	Status          string    `gorm:"type:varchar(20);not null;default:'pending'"`
	Attempts        int       `gorm:"not null;default:0"`
	AvailableAt     time.Time `gorm:"not null"`
	LeaseOwner      *string   `gorm:"type:varchar(128)"`
	LeaseExpiresAt  *time.Time
	ClaimedAt       *time.Time
	PublishedAt     *time.Time
	LastError       *string
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}
