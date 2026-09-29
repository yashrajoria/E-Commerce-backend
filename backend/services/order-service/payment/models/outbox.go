package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusPublished  = "published"
	OutboxStatusFailed     = "failed"
)

const (
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

func (OutboxEvent) TableName() string {
	return "payment_outbox_events"
}
