// Package pgtest gives repository tests a Postgres handle, or skips the test
// when none is configured.
package pgtest

import (
	"os"
	"testing"

	commondb "github.com/yashrajoria/common/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB opens TEST_DATABASE_URL, a database built by `migrate up`. Tests must
// create rows with random IDs and delete them in t.Cleanup — never TRUNCATE.
func DB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gdb, err := gorm.Open(postgres.Open(url), commondb.DefaultGormConfig())
	if err != nil {
		t.Fatal(err)
	}
	return gdb
}
