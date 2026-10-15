package db

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestConfigurePoolDefaults(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://user:pass@localhost:5432/test")
	if err != nil {
		t.Fatalf("failed to create sql.DB: %v", err)
	}
	defer db.Close()

	ConfigurePool(db, 3, 2)
	stats := db.Stats()
	if stats.MaxOpenConnections != 3 {
		t.Fatalf("expected MaxOpenConnections=3, got %d", stats.MaxOpenConnections)
	}
}

func TestConfigurePoolEnvOverrides(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://user:pass@localhost:5432/test")
	if err != nil {
		t.Fatalf("failed to create sql.DB: %v", err)
	}
	defer db.Close()

	os.Setenv("DB_MAX_OPEN_CONNS", "7")
	os.Setenv("DB_MAX_IDLE_CONNS", "4")
	defer os.Unsetenv("DB_MAX_OPEN_CONNS")
	defer os.Unsetenv("DB_MAX_IDLE_CONNS")

	ConfigurePool(db, 3, 2)
	stats := db.Stats()
	if stats.MaxOpenConnections != 7 {
		t.Fatalf("expected MaxOpenConnections=7, got %d", stats.MaxOpenConnections)
	}
}
