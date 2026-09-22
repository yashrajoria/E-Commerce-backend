package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"order-service/models"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func newDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=localhost user=test dbname=test", PreferSimpleProtocol: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	return db
}

func TestOutboxRepositoryClaimUsesLockingAndValidatesLimit(t *testing.T) {
	db := newDryRunDB(t)
	repository := &GormOutboxRepository{db: db}

	statement := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var events []models.OutboxEvent
		return tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND available_at <= ?", models.OutboxStatusPending, time.Now()).
			Order("created_at ASC").Limit(10).Find(&events)
	})
	if !strings.Contains(statement, "FOR UPDATE SKIP LOCKED") {
		t.Fatalf("claim query does not lock rows for safe claiming: %s", statement)
	}

	if _, err := repository.Claim(context.Background(), 0); err != ErrInvalidClaimLimit {
		t.Fatalf("expected invalid limit error, got %v", err)
	}
	if _, err := repository.ClaimWithLease(context.Background(), 1, "", time.Minute); err != ErrInvalidLease {
		t.Fatalf("expected invalid owner error, got %v", err)
	}
	if _, err := repository.ClaimWithLease(context.Background(), 1, "worker", 0); err != ErrInvalidLease {
		t.Fatalf("expected invalid lease duration error, got %v", err)
	}
}

func TestOutboxRepositoryOwnerUpdateReportsLostLease(t *testing.T) {
	db := newDryRunDB(t).Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true})
	repository := &GormOutboxRepository{db: db}
	if err := repository.MarkPublishedByOwner(context.Background(), uuid.New(), "worker"); err != ErrOutboxLeaseLost {
		t.Fatalf("expected lost lease error, got %v", err)
	}
	if err := repository.MarkFailedByOwner(context.Background(), uuid.New(), "worker", "send failed", time.Now()); err != ErrOutboxLeaseLost {
		t.Fatalf("expected lost lease error, got %v", err)
	}
}
