package services

import "regexp"

// validIdempotencyKey mirrors the async consumer charset:
// "<userID>:<hex-hash>" needs ':' alongside SQS-safe chars, capped at 128
// to match the varchar(128) unique index on payments/order idempotency keys.
var validIdempotencyKey = regexp.MustCompile(`^[a-zA-Z0-9_:\-]{1,128}$`)

// ValidateIdempotencyKey reports whether key is acceptable for checkout/payment.
func ValidateIdempotencyKey(key string) bool {
	if key == "" {
		return false
	}
	return validIdempotencyKey.MatchString(key)
}
