package repository_test

import (
	"context"
	"regexp"
	"shipping-service/models"
	"shipping-service/repository"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{})
	assert.NoError(t, err)
	return gormDB, mock
}

func TestCreate_Success(t *testing.T) {
	gormDB, mock := setupMockDB(t)
	repo := repository.NewGormShipmentRepository(gormDB)

	shipment := &models.Shipment{
		ID:           uuid.New(),
		OrderID:      "order-1",
		UserID:       "user-1",
		TrackingCode: "TRK001",
		Status:       models.ShipmentStatusCreated,
		WeightKg:     1.0,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "shipments"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(shipment.ID))
	mock.ExpectCommit()

	err := repo.Create(context.Background(), shipment)
	assert.NoError(t, err)
}

func TestFindByID_NotFound(t *testing.T) {
	gormDB, mock := setupMockDB(t)
	repo := repository.NewGormShipmentRepository(gormDB)

	id := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "shipments"`)).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{}))

	s, err := repo.FindByID(context.Background(), id)
	assert.Error(t, err)
	assert.Nil(t, s)
}

func TestFindByOrderID_Success(t *testing.T) {
	gormDB, mock := setupMockDB(t)
	repo := repository.NewGormShipmentRepository(gormDB)

	id := uuid.New()
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "order_id", "user_id", "status", "tracking_code", "weight_kg", "created_at", "updated_at"}).
		AddRow(id, "order-99", "user-1", models.ShipmentStatusCreated, "TRK099", 0.5, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "shipments"`)).
		WithArgs("order-99").
		WillReturnRows(rows)

	s, err := repo.FindByOrderID(context.Background(), "order-99")
	assert.NoError(t, err)
	assert.Equal(t, "order-99", s.OrderID)
}

func TestFindByTrackingCode_Success(t *testing.T) {
	gormDB, mock := setupMockDB(t)
	repo := repository.NewGormShipmentRepository(gormDB)

	id := uuid.New()
	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "order_id", "user_id", "status", "tracking_code", "weight_kg", "created_at", "updated_at"}).
		AddRow(id, "order-5", "user-5", models.ShipmentStatusInTransit, "TRK555", 2.0, now, now)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "shipments"`)).
		WithArgs("TRK555").
		WillReturnRows(rows)

	s, err := repo.FindByTrackingCode(context.Background(), "TRK555")
	assert.NoError(t, err)
	assert.Equal(t, models.ShipmentStatusInTransit, s.Status)
}

func TestUpdate_Success(t *testing.T) {
	gormDB, mock := setupMockDB(t)
	repo := repository.NewGormShipmentRepository(gormDB)

	shipment := &models.Shipment{
		ID:      uuid.New(),
		OrderID: "order-10",
		Status:  models.ShipmentStatusDelivered,
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "shipments"`)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err := repo.Update(context.Background(), shipment)
	assert.NoError(t, err)
}
