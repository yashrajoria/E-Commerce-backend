// Package models defines database models for user accounts, addresses, refresh tokens, and store tenants.
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
//
// BERP Documentation:
//
// Behavior:
//   - Serves as the central user account entity handling authentication, authorization, and customer profile data.
//   - Manages credential verification, password hashing, and email confirmation flows.
//   - Implements brute-force defense with lockout thresholds for both email verification and password logins.
//   - Stores customer contact details, role-based authorization level, and references to billing and shipping addresses.
//   - Supports soft-deletion through GORM DeletedAt to comply with GDPR/audit data retention policies.
//
// Exceptions:
//   - Unique constraint violation on Email (idx_users_email) for non-deleted records.
//   - Unique constraint violation on PhoneNumber (idx_users_phone_number) for non-deleted records.
//   - LoginLockedUntil error when failed password attempts exceed the allowed threshold.
//   - VerificationLockedUntil error when failed email verification code attempts exceed allowed threshold.
//   - Verification token expired error if VerificationExpiresAt is in the past.
//   - Foreign key constraint error if BillingAddressID or ShippingAddressID points to non-existent address records.
//
// Returns / Relations:
//   - Database Table: "users"
//   - Has One: BillingAddress (Address) via foreign key "BillingAddressID".
//   - Has One: ShippingAddress (Address) via foreign key "ShippingAddressID".
//   - Has Many: RefreshTokens ([]RefreshToken) via foreign key "UserID".
//   - Has Many: Addresses ([]Address) via foreign key "UserID".
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via PostgreSQL gen_random_uuid().
//   - Email (string): Unique user email address; partial unique index excludes soft-deleted rows.
//   - Password (string): Hashed user password (bcrypt/argon2); never stored in plaintext.
//   - Name (string): User's full name or display name.
//   - EmailVerified (bool): Flag indicating whether the user has validated their email address.
//   - VerificationCode (string): SHA-256 hex hash of the emailed OTP (size: 64); plaintext is never stored.
//   - VerificationAttempts (int): Consecutive failed verification attempts; resets to 0 on new code.
//   - VerificationExpiresAt (*time.Time): Expiration timestamp for the current verification code.
//   - VerificationLockedUntil (*time.Time): Lockout timestamp preventing further verification attempts until expired.
//   - LoginAttempts (int): Consecutive failed password login attempts for brute-force defense.
//   - LoginLockedUntil (*time.Time): Lockout timestamp preventing further login attempts until expired.
//   - StoreName (string): Merchant or vendor store name if user is a seller (max 100 chars).
//   - Role (string): Authorization role: 'user', 'admin', 'merchant' (defaults to 'user', max 50 chars).
//   - PhoneNumber (*string): Optional unique phone number; partial unique index excludes soft-deleted rows.
//   - BillingAddressID (*uuid.UUID): Foreign key reference to primary billing Address.
//   - ShippingAddressID (*uuid.UUID): Foreign key reference to primary shipping Address.
//   - BillingAddress (Address): Associated billing Address relation struct.
//   - ShippingAddress (Address): Associated shipping Address relation struct.
//   - CreatedAt (time.Time): Timestamp of user registration.
//   - UpdatedAt (time.Time): Timestamp of last user profile/security update.
//   - DeletedAt (gorm.DeletedAt): Soft-delete timestamp for account deactivation.
//
// Example:
//
//	phone := "+1-555-0199"
//	user := models.User{
//	    Email:         "jane.doe@example.com",
//	    Password:      "$2a$12$e8k...hashed_password...", // Hashed via bcrypt
//	    Name:          "Jane Doe",
//	    EmailVerified: false,
//	    Role:          "user",
//	    PhoneNumber:   &phone,
//	}
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
	VerificationExpiresAt   *time.Time `gorm:""`
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

// Address represents a physical postal address belonging to a user.
// It is designated for either billing or shipping purposes.
//
// BERP Documentation:
//
// Behavior:
//   - Stores structured street address information for tax calculation, invoice generation, and package delivery.
//   - Enforces address classification ('billing' or 'shipping') via database check constraint.
//   - Associated with a single User account and referenced by user profile defaults.
//   - Supports soft-delete to preserve delivery histories for existing orders.
//
// Exceptions:
//   - Database check constraint error if Type is not 'billing' or 'shipping'.
//   - Not-null constraint violation if Street, City, State, PostalCode, or Country are omitted.
//   - Foreign key constraint error if UserID does not reference an existing User record.
//
// Returns / Relations:
//   - Database Table: "addresses"
//   - Belongs To: User via foreign key "UserID".
//   - Referenced By: User.BillingAddressID and User.ShippingAddressID.
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via gen_random_uuid().
//   - UserID (uuid.UUID): Foreign key identifying the owning User (indexed).
//   - Type (string): Address role: strictly 'billing' or 'shipping' (checked in DB, max 20 chars).
//   - Street (string): Street address line (including number, street, apt/suite).
//   - City (string): Municipality or city name.
//   - State (string): State, province, or administrative region.
//   - PostalCode (string): Postal code or ZIP code.
//   - Country (string): Country name or ISO country code.
//   - CreatedAt (time.Time): Address creation timestamp.
//   - UpdatedAt (time.Time): Address update timestamp.
//   - DeletedAt (gorm.DeletedAt): Soft-delete timestamp.
//
// Example:
//
//	address := models.Address{
//	    UserID:     user.ID,
//	    Type:       "shipping",
//	    Street:     "742 Evergreen Terrace",
//	    City:       "Springfield",
//	    State:      "OR",
//	    PostalCode: "97477",
//	    Country:    "USA",
//	}
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
//
// BERP Documentation:
//
// Behavior:
//   - Grants new short-lived access tokens without requiring the user to re-enter credentials.
//   - Uses token rotation: each refresh request invalidates the old token and issues a new one.
//   - Uses FamilyID grouping: every token descended from the same initial login shares a FamilyID.
//   - Detects token theft/replay: if an already-rotated or revoked token is reused, all tokens
//     in that FamilyID are immediately revoked to terminate the attacker's session lineage.
//
// Exceptions:
//   - Unique constraint violation if TokenID (JTI) collision occurs.
//   - Token revoked error if Revoked is true.
//   - Token expired error if current timestamp exceeds ExpiresAt.
//   - Family compromised exception if an already consumed token is presented again.
//   - Foreign key constraint error if UserID does not reference an existing User.
//
// Returns / Relations:
//   - Database Table: "refresh_tokens"
//   - Belongs To: User via foreign key "UserID".
//   - Token Family: Grouped by FamilyID for complete session hierarchy invalidation.
//
// Parameters / Properties:
//   - ID (uuid.UUID): Primary key, auto-generated via gen_random_uuid().
//   - TokenID (string): Unique cryptographic token identifier / JWT ID (jti); unique constraint.
//   - UserID (uuid.UUID): Foreign key identifying the token owner (indexed).
//   - FamilyID (uuid.UUID): Identifier common to all tokens derived from the same login session (indexed).
//   - Revoked (bool): Flag indicating if this token has been invalidated (default: false).
//   - RevokedAt (*time.Time): Nullable timestamp recording when revocation occurred.
//   - ExpiresAt (time.Time): Absolute expiration timestamp after which the token cannot be used (indexed).
//   - CreatedAt (time.Time): Token issuance timestamp.
//
// Example:
//
//	familyID := uuid.New()
//	refreshToken := models.RefreshToken{
//	    TokenID:   "rt_9f83a21b4e5c6d7e",
//	    UserID:    user.ID,
//	    FamilyID:  familyID,
//	    Revoked:   false,
//	    ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7-day validity
//	}
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
