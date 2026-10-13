package db

import (
	"net/url"
	"testing"
)

func TestPostgresDSNEscapesCredentialsAndDatabase(t *testing.T) {
	dsn := PostgresDSN("db.example", "5432", "user@example", "p@ss:word/with?chars", "shop/db", "require", "Asia/Kolkata")

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if parsed.User.Username() != "user@example" {
		t.Fatalf("username = %q", parsed.User.Username())
	}
	password, ok := parsed.User.Password()
	if !ok || password != "p@ss:word/with?chars" {
		t.Fatalf("password was not preserved through escaping")
	}
	if parsed.Path != "/shop/db" {
		t.Fatalf("path = %q, want /shop/db", parsed.Path)
	}
	if parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("sslmode = %q", parsed.Query().Get("sslmode"))
	}
}
