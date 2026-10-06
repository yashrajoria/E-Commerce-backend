package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User is the single unified identity + profile model for the shared `users`
// table. It merges the columns previously owned separately by auth-service
// (credentials, verification, lockout) and user-service (profile, addresses,
// soft delete). One service, one model, one migration — no more split-brain
// AutoMigrate over the same table.
type User struct {
	ID       uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Email    string    `gorm:"not null;uniqueIndex:idx_users_email,where:deleted_at IS NULL"`
	Password string    `gorm:"not null"`
	Name     string    `gorm:"not null"`

	// Identity columns (ex-auth-service).
	EmailVerified bool `gorm:"default:false"`
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

	// Profile columns (ex-user-service).
	StoreName         string  `gorm:"size:100"`
	Role              string  `gorm:"type:varchar(50);default:'user'"`
	PhoneNumber       *string `gorm:"uniqueIndex:idx_users_phone_number,where:deleted_at IS NULL"`
	BillingAddressID  *uuid.UUID
	ShippingAddressID *uuid.UUID
	BillingAddress    Address `gorm:"foreignKey:BillingAddressID"`
	ShippingAddress   Address `gorm:"foreignKey:ShippingAddressID"`

	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type Address struct {
	ID         uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index"`
	Type       string         `gorm:"type:varchar(20);check:type IN ('billing', 'shipping')"`
	Street     string         `gorm:"not null"`
	City       string         `gorm:"not null"`
	State      string         `gorm:"not null"`
	PostalCode string         `gorm:"not null"`
	Country    string         `gorm:"not null"`
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// RefreshToken model stores issued refresh tokens for rotation and revocation.
// FamilyID is shared by every token descended from the same login, so that
// reuse of an already-rotated token (a sign of theft) can revoke the whole
// lineage instead of just the one token.
type RefreshToken struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	TokenID   string     `gorm:"unique;not null"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	FamilyID  uuid.UUID  `gorm:"type:uuid;not null;index"`
	Revoked   bool       `gorm:"default:false"`
	RevokedAt *time.Time `gorm:""`
	ExpiresAt time.Time  `gorm:"not null;index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
}
