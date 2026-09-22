package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Order struct {
	ID             uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderNumber    string    `gorm:"uniqueIndex;not null"`
	IdempotencyKey *string   `gorm:"type:varchar(128);uniqueIndex"`
	UserID         uuid.UUID `gorm:"type:uuid;not null;index"`
	Amount         int       `gorm:"not null"`
	CouponCode     string    `gorm:"type:varchar(50)"`
	DiscountAmount int       `gorm:"default:0"`
	Status         string    `gorm:"type:varchar(20);not null;default:'pending_payment'"`
	CanceledAt     *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	OrderItems     []OrderItem    `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE"`
}

type OrderItem struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	OrderID   uuid.UUID `gorm:"type:uuid;not null;index"`
	ProductID uuid.UUID `gorm:"type:uuid;not null"`
	Quantity  int       `gorm:"not null"`
	Price     int       `gorm:"not null"`
}

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusPublished  = "published"
	OutboxStatusFailed     = "failed"
)

const (
	OutboxDestinationSQS = "sqs"
	OutboxDestinationSNS = "sns"
)

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
