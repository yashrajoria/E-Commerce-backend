package services

import (
	"errors"
	"regexp"
	"unicode"
)

var (
	ErrPasswordTooShort      = errors.New("password must be at least 8 characters long")
	ErrPasswordNoUpper       = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower       = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoNumber      = errors.New("password must contain at least one number")
	ErrPasswordNoSpecial     = errors.New("password must contain at least one special character")
	ErrPasswordCommon        = errors.New("password is too common")
	ErrPasswordSequential    = errors.New("password contains sequential characters")
	ErrPasswordRepeating     = errors.New("password contains repeating characters")
)

// PasswordValidator validates passwords against security requirements
type PasswordValidator struct {
	minLength      int
	requireUpper   bool
	requireLower   bool
	requireNumber  bool
	requireSpecial bool
	commonPasswords map[string]bool
}

// NewPasswordValidator creates a new password validator with default settings
func NewPasswordValidator() *PasswordValidator {
	return &PasswordValidator{
		minLength:      8,
		requireUpper:   true,
		requireLower:   true,
		requireNumber:  true,
		requireSpecial: true,
		commonPasswords: map[string]bool{
			"password": true,
			"123456":   true,
			"qwerty":   true,
			"admin":    true,
			"welcome":  true,
		},
	}
}

// ValidatePassword checks if a password meets all security requirements
func (pv *PasswordValidator) ValidatePassword(password string) error {
	if len(password) < pv.minLength {
		return ErrPasswordTooShort
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool
	var prevChar rune
	var repeatCount int

	for i, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}

		// Check for repeating characters
		if i > 0 && char == prevChar {
			repeatCount++
			if repeatCount >= 3 {
				return ErrPasswordRepeating
			}
		} else {
			repeatCount = 1
		}

		// Check for sequential characters
		if i > 0 && (char == prevChar+1 || char == prevChar-1) {
			return ErrPasswordSequential
		}

		prevChar = char
	}

	if pv.requireUpper && !hasUpper {
		return ErrPasswordNoUpper
	}
	if pv.requireLower && !hasLower {
		return ErrPasswordNoLower
	}
	if pv.requireNumber && !hasNumber {
		return ErrPasswordNoNumber
	}
	if pv.requireSpecial && !hasSpecial {
		return ErrPasswordNoSpecial
	}

	// Check against common passwords
	if pv.commonPasswords[password] {
		return ErrPasswordCommon
	}

	return nil
}

// IsPasswordStrong checks if a password meets minimum security requirements
func IsPasswordStrong(password string) bool {
	validator := NewPasswordValidator()
	return validator.ValidatePassword(password) == nil
} 