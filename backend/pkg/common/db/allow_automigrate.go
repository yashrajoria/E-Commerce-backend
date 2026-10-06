package db

import (
	"os"
	"strconv"
	"strings"
)

// AllowAutoMigrate returns true unless ALLOW_AUTO_MIGRATE is explicitly false/0/no.
func AllowAutoMigrate() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ALLOW_AUTO_MIGRATE")))
	if v == "" {
		return true
	}
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return true
}
