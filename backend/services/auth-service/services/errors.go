package services

import "errors"

// Sentinel errors so controllers can dispatch HTTP status via errors.Is
// instead of matching on err.Error() substrings (fragile against message wording changes).
var (
	ErrEmailAlreadyExists      = errors.New("email already exists")
	ErrUserNotFound            = errors.New("user not found")
	ErrInvalidVerificationCode = errors.New("invalid verification code")
	ErrEmailAlreadyVerified    = errors.New("email already verified")
)
