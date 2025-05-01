package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID       uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email    string    `gorm:"unique;not null"`
	Password string    `gorm:"not null"`
	// PhoneNumber       *string   `gorm:"unique"`
	EmailVerified     bool   `gorm:"default:false"`
	VerificationCode  string `gorm:"size:6"`
	Role              string `gorm:"type:varchar(50);default:'user'"`
	BillingAddressID  *uuid.UUID
	ShippingAddressID *uuid.UUID
	BillingAddress    Address   `gorm:"foreignKey:BillingAddressID"`
	ShippingAddress   Address   `gorm:"foreignKey:ShippingAddressID"`
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

// Address model
type Address struct {
	ID         uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID     uuid.UUID `gorm:"type:uuid;not null"`
	Type       string    `gorm:"type:varchar(20);check:type IN ('billing', 'shipping')"`
	Street     string    `gorm:"not null"`
	City       string    `gorm:"not null"`
	State      string    `gorm:"not null"`
	PostalCode string    `gorm:"not null"`
	Country    string    `gorm:"not null"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
	UpdatedAt  time.Time `gorm:"autoUpdateTime"`
}

// Migrate function for auto migration
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &Address{})
}
