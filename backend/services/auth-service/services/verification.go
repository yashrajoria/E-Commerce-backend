package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
)

// GenerateRandomCode returns a numeric code of the given length, used for
// email verification. Uses crypto/rand so codes aren't guessable.
func GenerateRandomCode(length int) string {
	code := ""
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// Fallback to 0 in the unlikely event of entropy failure
			code += "0"
			continue
		}
		code += n.String()
	}
	return code
}

// hashVerificationCode hashes a verification code for storage, so a DB read
// alone (backup leak, SQLi elsewhere) doesn't hand over a live, usable code.
// The plaintext code is only ever transmitted once, in the email itself.
func hashVerificationCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
