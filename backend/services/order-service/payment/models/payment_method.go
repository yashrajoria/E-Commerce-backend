package models

import (
	"time"

	"github.com/google/uuid"
)

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

func (UserPaymentMethod) TableName() string {
	return "user_payment_methods"
}
