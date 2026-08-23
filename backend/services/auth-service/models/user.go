package models

import (
	"time"

	"github.com/google/uuid"
	commondb "github.com/yashrajoria/common/db"
	"gorm.io/gorm"
)

// User model
type User struct {
	ID            uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email         string    `gorm:"unique;not null"`
	Password      string    `gorm:"not null"`
	Name          string    `gorm:"not null"`
	EmailVerified bool      `gorm:"default:false"`
	// VerificationCode stores the SHA-256 hex hash of the emailed code, not
	// the plaintext (a DB read alone shouldn't hand over a usable code).
	VerificationCode string `gorm:"size:64"`
	// VerificationAttempts counts consecutive failed verify-email attempts since the
	// code was last (re)issued; reset to 0 whenever a new code is generated.
	VerificationAttempts    int        `gorm:"default:0"`
	VerificationLockedUntil *time.Time `gorm:""`
	// LoginAttempts/LoginLockedUntil mirror the verification-code lockout
	// above, but for password login: brute-force protection independent of
	// the gateway's per-IP rate limit (which a distributed attacker can
	// route around across many IPs).
	LoginAttempts    int        `gorm:"default:0"`
	LoginLockedUntil *time.Time `gorm:""`
	StoreName        string     `gorm:"size:100"`
	Role             string     `gorm:"type:varchar(50);default:'user'"`
	CreatedAt        time.Time  `gorm:"autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"autoUpdateTime"`
}

// RefreshToken model stores issued refresh tokens for rotation and revocation.
// FamilyID is shared by every token descended from the same login, so that
// reuse of an already-rotated token (a sign of theft) can revoke the whole
// lineage instead of just the one token.
type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	TokenID   string    `gorm:"unique;not null"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	FamilyID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Revoked   bool      `gorm:"default:false"`
	ExpiresAt time.Time `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// Migrate function for auto migration (gated by ALLOW_AUTO_MIGRATE).
func Migrate(db *gorm.DB) error {
	if !commondb.AllowAutoMigrate() {
		return nil
	}
	return db.AutoMigrate(&User{}, &RefreshToken{})
}
