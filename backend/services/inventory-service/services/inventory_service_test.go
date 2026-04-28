package services

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yashrajoria/inventory-service/models"
)

// MockInventoryRepository is a mock of the repository interface
type MockInventoryRepository struct {
	mock.Mock
}

func (m *MockInventoryRepository) Get(ctx context.Context, productID string) (*models.Inventory, error) {
	args := m.Called(ctx, productID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Inventory), args.Error(1)
}

func (m *MockInventoryRepository) Set(ctx context.Context, inv *models.Inventory) error {
	return m.Called(ctx, inv).Error(0)
}

func (m *MockInventoryRepository) Update(ctx context.Context, productID string, updates map[string]interface{}) error {
	return m.Called(ctx, productID, updates).Error(0)
}

func (m *MockInventoryRepository) ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return m.Called(ctx, orderID, items).Error(0)
}

func (m *MockInventoryRepository) ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return m.Called(ctx, orderID, items).Error(0)
}

func (m *MockInventoryRepository) ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	return m.Called(ctx, orderID, items).Error(0)
}

func (m *MockInventoryRepository) CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error) {
	args := m.Called(ctx, productID, quantity)
	return args.Get(0).(*models.StockCheckResult), args.Error(1)
}

func (m *MockInventoryRepository) ListAll(ctx context.Context, limit int32, exclusiveStartKey map[string]types.AttributeValue) ([]models.Inventory, map[string]types.AttributeValue, error) {
	// Not needed for this test
	return nil, nil, nil
}

func TestReserveStock_TransactionalSuccess(t *testing.T) {
	repo := new(MockInventoryRepository)
	service := NewInventoryService(repo, nil)

	req := &models.ReserveRequest{
		OrderID: "order-123",
		Items: []models.ReserveItem{
			{ProductID: "prod-1", Quantity: 2},
			{ProductID: "prod-2", Quantity: 1},
		},
	}

	repo.On("ReserveAll", mock.Anything, "order-123", req.Items).Return(nil)

	results, err := service.ReserveStock(context.Background(), req)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "prod-1", results[0].ProductID)
	repo.AssertExpectations(t)
}

func TestReserveStock_TransactionalFailure(t *testing.T) {
	repo := new(MockInventoryRepository)
	service := NewInventoryService(repo, nil)

	req := &models.ReserveRequest{
		OrderID: "order-123",
		Items: []models.ReserveItem{
			{ProductID: "prod-1", Quantity: 100},
		},
	}

	// Simulate insufficient stock error from the transactional repo
	repo.On("ReserveAll", mock.Anything, "order-123", req.Items).Return(errors.New("insufficient stock"))

	results, err := service.ReserveStock(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Equal(t, "insufficient stock", err.Error())
	repo.AssertExpectations(t)
}
