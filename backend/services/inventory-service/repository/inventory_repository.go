package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/yashrajoria/inventory-service/models"
)

var (
	ErrNotFound          = errors.New("inventory record not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

// InventoryRepository defines the interface for inventory data access
type InventoryRepository interface {
	Get(ctx context.Context, productID string) (*models.Inventory, error)
	Set(ctx context.Context, inv *models.Inventory) error
	Update(ctx context.Context, productID string, updates map[string]interface{}) error
	Reserve(ctx context.Context, productID string, quantity int) error
	Release(ctx context.Context, productID string, quantity int) error
	Confirm(ctx context.Context, productID string, quantity int) error
	CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error)
}

// DynamoInventoryRepository implements InventoryRepository using DynamoDB
type DynamoInventoryRepository struct {
	client *dynamodb.Client
	table  string
}

// NewDynamoInventoryRepository creates a new DynamoDB backed inventory repository
func NewDynamoInventoryRepository(client *dynamodb.Client, table string) *DynamoInventoryRepository {
	return &DynamoInventoryRepository{client: client, table: table}
}

type ddbInventory struct {
	ProductID string `dynamodbav:"product_id"`
	Available int    `dynamodbav:"available"`
	Reserved  int    `dynamodbav:"reserved"`
	Threshold int    `dynamodbav:"threshold"`
	UpdatedAt string `dynamodbav:"updated_at"`
}

func (r *DynamoInventoryRepository) Get(ctx context.Context, productID string) (*models.Inventory, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": productID})
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	out, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &r.table,
		Key:       key,
	})
	if err != nil {
		return nil, fmt.Errorf("dynamodb GetItem failed: %w", err)
	}
	if len(out.Item) == 0 {
		return nil, ErrNotFound
	}

	var di ddbInventory
	if err := attributevalue.UnmarshalMap(out.Item, &di); err != nil {
		return nil, fmt.Errorf("unmarshal item: %w", err)
	}

	inv := &models.Inventory{
		ProductID: di.ProductID,
		Available: di.Available,
		Reserved:  di.Reserved,
		Threshold: di.Threshold,
	}
	if t, err := time.Parse(time.RFC3339, di.UpdatedAt); err == nil {
		inv.UpdatedAt = t
	}
	return inv, nil
}

func (r *DynamoInventoryRepository) Set(ctx context.Context, inv *models.Inventory) error {
	di := ddbInventory{
		ProductID: inv.ProductID,
		Available: inv.Available,
		Reserved:  inv.Reserved,
		Threshold: inv.Threshold,
		UpdatedAt: inv.UpdatedAt.Format(time.RFC3339),
	}

	item, err := attributevalue.MarshalMap(di)
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}

	_, err = r.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &r.table,
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("dynamodb PutItem failed: %w", err)
	}
	return nil
}

func (r *DynamoInventoryRepository) Update(ctx context.Context, productID string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	expr := "SET "
	exprVals := make(map[string]types.AttributeValue)
	exprNames := make(map[string]string)
	i := 0
	for k, v := range updates {
		ph := fmt.Sprintf(":v%d", i)
		namePh := fmt.Sprintf("#f%d", i)
		if i > 0 {
			expr += ", "
		}
		expr += fmt.Sprintf("%s = %s", namePh, ph)
		exprNames[namePh] = k
		av, err := attributevalue.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshal update value: %w", err)
		}
		exprVals[ph] = av
		i++
	}

	key, err := attributevalue.MarshalMap(map[string]string{"product_id": productID})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &r.table,
		Key:                       key,
		UpdateExpression:          &expr,
		ExpressionAttributeValues: exprVals,
		ExpressionAttributeNames:  exprNames,
	})
	if err != nil {
		return fmt.Errorf("update item failed: %w", err)
	}
	return nil
}

// Reserve atomically decrements available and increments reserved
func (r *DynamoInventoryRepository) Reserve(ctx context.Context, productID string, quantity int) error {
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": productID})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	expr := "SET #avail = #avail - :qty, #resv = #resv + :qty, updated_at = :now"
	condExpr := "#avail >= :qty"

	qtyAV, _ := attributevalue.Marshal(quantity)
	nowAV, _ := attributevalue.Marshal(time.Now().UTC().Format(time.RFC3339))

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &r.table,
		Key:                 key,
		UpdateExpression:    &expr,
		ConditionExpression: &condExpr,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":qty": qtyAV,
			":now": nowAV,
		},
		ExpressionAttributeNames: map[string]string{
			"#avail": "available",
			"#resv":  "reserved",
		},
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return ErrInsufficientStock
		}
		return fmt.Errorf("reserve failed: %w", err)
	}
	return nil
}

// Release atomically increments available and decrements reserved
func (r *DynamoInventoryRepository) Release(ctx context.Context, productID string, quantity int) error {
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": productID})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	expr := "SET #avail = #avail + :qty, #resv = #resv - :qty, updated_at = :now"
	condExpr := "#resv >= :qty"

	qtyAV, _ := attributevalue.Marshal(quantity)
	nowAV, _ := attributevalue.Marshal(time.Now().UTC().Format(time.RFC3339))

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &r.table,
		Key:                 key,
		UpdateExpression:    &expr,
		ConditionExpression: &condExpr,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":qty": qtyAV,
			":now": nowAV,
		},
		ExpressionAttributeNames: map[string]string{
			"#avail": "available",
			"#resv":  "reserved",
		},
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("cannot release more than reserved")
		}
		return fmt.Errorf("release failed: %w", err)
	}
	return nil
}

// Confirm permanently deducts reserved stock (payment succeeded)
func (r *DynamoInventoryRepository) Confirm(ctx context.Context, productID string, quantity int) error {
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": productID})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	expr := "SET #resv = #resv - :qty, updated_at = :now"
	condExpr := "#resv >= :qty"

	qtyAV, _ := attributevalue.Marshal(quantity)
	nowAV, _ := attributevalue.Marshal(time.Now().UTC().Format(time.RFC3339))

	_, err = r.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:           &r.table,
		Key:                 key,
		UpdateExpression:    &expr,
		ConditionExpression: &condExpr,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":qty": qtyAV,
			":now": nowAV,
		},
		ExpressionAttributeNames: map[string]string{
			"#resv": "reserved",
		},
	})
	if err != nil {
		var ccf *types.ConditionalCheckFailedException
		if errors.As(err, &ccf) {
			return fmt.Errorf("cannot confirm more than reserved")
		}
		return fmt.Errorf("confirm failed: %w", err)
	}
	return nil
}

// CheckStock returns stock availability for a product
func (r *DynamoInventoryRepository) CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error) {
	inv, err := r.Get(ctx, productID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &models.StockCheckResult{
				ProductID:    productID,
				Available:    0,
				Reserved:     0,
				Requested:    quantity,
				IsSufficient: false,
			}, nil
		}
		return nil, err
	}

	return &models.StockCheckResult{
		ProductID:    productID,
		Available:    inv.Available,
		Reserved:     inv.Reserved,
		Requested:    quantity,
		IsSufficient: inv.Available >= quantity,
	}, nil
}
