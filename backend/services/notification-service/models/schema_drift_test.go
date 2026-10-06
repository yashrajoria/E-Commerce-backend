package models_test

import (
	"os"
	"strings"
	"testing"

	"notification-service/models"

	commondb "github.com/yashrajoria/common/db"
)

// Keep the model list in sync with database.Connect.
func TestAutoMigrateAddsNothingToMigratedSchema(t *testing.T) {
	url := os.Getenv("SCHEMA_DRIFT_DB_URL")
	if url == "" {
		t.Skip("set SCHEMA_DRIFT_DB_URL to a database built only by `migrate up`")
	}
	diff, err := commondb.AutoMigrateDrift(url, &models.NotificationLog{}, &models.NotificationEvent{})
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) > 0 {
		t.Fatalf("migrations and models disagree:\n%s", strings.Join(diff, "\n"))
	}
}
