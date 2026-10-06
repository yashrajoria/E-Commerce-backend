package db

import (
	"context"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Needs a real Postgres (ctid): set TEST_DATABASE_URL, e.g. the CI service container.
func TestPurgeDeletesOnlyMatchingRowsAcrossBatches(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gdb, err := gorm.Open(postgres.Open(url), DefaultGormConfig())
	if err != nil {
		t.Fatal(err)
	}
	gdb.Exec("DROP TABLE IF EXISTS purge_test")
	gdb.Exec("CREATE TABLE purge_test (id serial PRIMARY KEY, old boolean NOT NULL)")
	defer gdb.Exec("DROP TABLE IF EXISTS purge_test")
	// 2 batches of old rows plus a remainder, and 3 rows that must survive.
	gdb.Exec("INSERT INTO purge_test (old) SELECT true FROM generate_series(1, ?)", purgeBatch*2+7)
	gdb.Exec("INSERT INTO purge_test (old) SELECT false FROM generate_series(1, 3)")

	purge(context.Background(), gdb, PurgeJob{Table: "purge_test", Where: "old"})

	var left, old int64
	gdb.Raw("SELECT count(*), count(*) FILTER (WHERE old) FROM purge_test").Row().Scan(&left, &old)
	if left != 3 || old != 0 {
		t.Fatalf("left=%d old=%d, want 3 and 0", left, old)
	}
}
