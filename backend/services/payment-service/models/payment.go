package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Payment struct {
	Payment_ID         uuid.UUID `gorm:"type:uuid;json default:gen_random_uuid();primaryKey"`
	OrderID            uuid.UUID `gorm:"type:uuid;index;not null"`
	UserID             uuid.UUID `gorm:"type:uuid;index;not null"`
	Amount             int       `gorm:"not null"` // in cents/paise
	Currency           string    `gorm:"type:varchar(10);not null"`
	Status             string    `gorm:"type:varchar(20);not null"`
	CheckoutURL        *string   `gorm:"type:varchar(1024)"` // Nullable URL
	StripePaymentID    *string   `gorm:"uniqueIndex"`
	StripeEventPayload *string   `gorm:"type:jsonb"` // Optional: for audit and debugging
	SucceededAt        *time.Time
	FailedAt           *time.Time
	CreatedAt          time.Time      `gorm:"autoCreateTime"`
	UpdatedAt          time.Time      `gorm:"autoUpdateTime"`
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}
