package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID               uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email            string    `gorm:"unique;not null"`
	Password         string    `gorm:"not null"`
	EmailVerified    bool      `gorm:"default:false"`
	VerificationCode string    `gorm:"size:6"`
	Role             string    `gorm:"type:varchar(50);default:'user'"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

// Migrate function for auto migration
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{})
}
