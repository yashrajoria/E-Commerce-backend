package models

import (
	"time"

	"github.com/google/uuid"
	commondb "github.com/yashrajoria/common/db"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID                        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email                     string    `gorm:"unique;not null"`
	Password                  string    `gorm:"not null"`
	Name                      string    `gorm:"not null"`
	EmailVerified             bool      `gorm:"default:false"`
	VerificationCode          string    `gorm:"size:6"`
	VerificationCodeExpiresAt time.Time `gorm:""`
	StoreName                 string    `gorm:"size:100"`
	Role                      string    `gorm:"type:varchar(50);default:'user'"`
	CreatedAt                 time.Time `gorm:"autoCreateTime"`
	UpdatedAt                 time.Time `gorm:"autoUpdateTime"`
}

// RefreshToken model stores issued refresh tokens for rotation and revocation
type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	TokenID   string     `gorm:"unique;not null"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	Revoked   bool       `gorm:"default:false"`
	RevokedAt *time.Time `gorm:""`
	ExpiresAt time.Time  `gorm:"not null;index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
}

// Migrate function for auto migration (gated by ALLOW_AUTO_MIGRATE).
func Migrate(db *gorm.DB) error {
	if !commondb.AllowAutoMigrate() {
		return nil
	}
	return db.AutoMigrate(&User{}, &RefreshToken{})
}
