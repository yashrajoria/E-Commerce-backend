package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/yashrajoria/inventory-service/models"
)

// inventoryTxnToken builds a DynamoDB ClientRequestToken (max 36 chars).
// Op must be a single letter so reserve/release/confirm stay distinct for the same order.
func inventoryTxnToken(op, orderID string) string {
	compact := strings.ReplaceAll(orderID, "-", "")
	if len(compact) > 35 {
		compact = compact[:35]
	}
	return op + compact
}

var (
	ErrNotFound          = errors.New("inventory record not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

// InventoryRepository defines the interface for inventory data access
type InventoryRepository interface {
	Get(ctx context.Context, productID string) (*models.Inventory, error)
	Set(ctx context.Context, inv *models.Inventory) error
	Update(ctx context.Context, productID string, updates map[string]interface{}) error
	ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error
	CheckStock(ctx context.Context, productID string, quantity int) (*models.StockCheckResult, error)
	ListAll(ctx context.Context, limit int32, exclusiveStartKey map[string]types.AttributeValue) ([]models.Inventory, map[string]types.AttributeValue, error)
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
	ProductID         string         `dynamodbav:"id"`
	Available         int            `dynamodbav:"available"`
	Reserved          int            `dynamodbav:"reserved"`
	Threshold         int            `dynamodbav:"threshold"`
	OrderReservations map[string]int `dynamodbav:"order_reservations,omitempty"`
	UpdatedAt         string         `dynamodbav:"updated_at"`
}

func (r *DynamoInventoryRepository) Get(ctx context.Context, productID string) (*models.Inventory, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"id": productID})
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
		ProductID:         di.ProductID,
		Available:         di.Available,
		Reserved:          di.Reserved,
		Threshold:         di.Threshold,
		OrderReservations: di.OrderReservations,
	}
	if t, err := time.Parse(time.RFC3339, di.UpdatedAt); err == nil {
		inv.UpdatedAt = t
	}
	return inv, nil
}

func (r *DynamoInventoryRepository) Set(ctx context.Context, inv *models.Inventory) error {
	orderRes := inv.OrderReservations
	if orderRes == nil {
		orderRes = map[string]int{}
	}
	di := ddbInventory{
		ProductID:         inv.ProductID,
		Available:         inv.Available,
		Reserved:          inv.Reserved,
		Threshold:         inv.Threshold,
		OrderReservations: orderRes,
		UpdatedAt:         inv.UpdatedAt.Format(time.RFC3339),
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

	key, err := attributevalue.MarshalMap(map[string]string{"id": productID})
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

// ReserveAll atomically and idempotently reserves stock for multiple items in a transaction.
func (r *DynamoInventoryRepository) ReserveAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	transactItems := make([]types.TransactWriteItem, 0, len(items))
	now := time.Now().UTC().Format(time.RFC3339)

	for _, item := range items {
		key, _ := attributevalue.MarshalMap(map[string]string{"id": item.ProductID})
		qtyAV, _ := attributevalue.Marshal(item.Quantity)
		nowAV, _ := attributevalue.Marshal(now)

		// Ensure order_reservations map exists (DynamoDB nested SET requires parent map),
		// then atomically decrement available and record per-order reservation.
		expr := "SET #avail = #avail - :qty, #resv = #resv + :qty, " +
			"#order_resv = if_not_exists(#order_resv, :empty), " +
			"#order_resv.#orderID = :qty, updated_at = :now"
		cond := "#avail >= :qty AND attribute_not_exists(#order_resv.#orderID)"

		emptyMap, _ := attributevalue.Marshal(map[string]int{})

		transactItems = append(transactItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName:           &r.table,
				Key:                 key,
				UpdateExpression:    &expr,
				ConditionExpression: &cond,
				ExpressionAttributeNames: map[string]string{
					"#avail":      "available",
					"#resv":       "reserved",
					"#order_resv": "order_reservations",
					"#orderID":    orderID,
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":qty":   qtyAV,
					":now":   nowAV,
					":empty": emptyMap,
				},
			},
		})
	}

	// ClientRequestToken makes the whole transaction idempotent on retries (AWS best practice).
	token := inventoryTxnToken("v", orderID) // "v" = reserve (distinct from release/confirm)
	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems:      transactItems,
		ClientRequestToken: &token,
	})
	if err != nil {
		var ccf *types.TransactionCanceledException
		if errors.As(err, &ccf) {
			for _, reason := range ccf.CancellationReasons {
				if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
					return fmt.Errorf("insufficient stock or duplicate reservation for order=%s", orderID)
				}
			}
		}
		return fmt.Errorf("transact reserve failed: %w", err)
	}
	return nil
}

// ReleaseAll atomically releases reservations for multiple items in a transaction.
func (r *DynamoInventoryRepository) ReleaseAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	transactItems := make([]types.TransactWriteItem, 0, len(items))
	now := time.Now().UTC().Format(time.RFC3339)

	for _, item := range items {
		key, _ := attributevalue.MarshalMap(map[string]string{"id": item.ProductID})
		qtyAV, _ := attributevalue.Marshal(item.Quantity)
		nowAV, _ := attributevalue.Marshal(now)

		// Add back to available, remove from reserved and the order tracking map
		expr := "SET #avail = #avail + :qty, #resv = #resv - :qty, updated_at = :now REMOVE #order_resv.#orderID"
		cond := "attribute_exists(#order_resv.#orderID)"

		transactItems = append(transactItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName:           &r.table,
				Key:                 key,
				UpdateExpression:    &expr,
				ConditionExpression: &cond,
				ExpressionAttributeNames: map[string]string{
					"#avail":      "available",
					"#resv":       "reserved",
					"#order_resv": "order_reservations",
					"#orderID":    orderID,
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":qty": qtyAV,
					":now": nowAV,
				},
			},
		})
	}

	token := inventoryTxnToken("r", orderID)
	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems:      transactItems,
		ClientRequestToken: &token,
	})
	if err != nil {
		return fmt.Errorf("transact release failed: %w", err)
	}
	return nil
}

// ConfirmAll permanently deducts reserved stock for multiple items in a transaction.
func (r *DynamoInventoryRepository) ConfirmAll(ctx context.Context, orderID string, items []models.ReserveItem) error {
	transactItems := make([]types.TransactWriteItem, 0, len(items))
	now := time.Now().UTC().Format(time.RFC3339)

	for _, item := range items {
		key, _ := attributevalue.MarshalMap(map[string]string{"id": item.ProductID})
		qtyAV, _ := attributevalue.Marshal(item.Quantity)
		nowAV, _ := attributevalue.Marshal(now)

		// Just remove from reserved and the order tracking map (stock already decremented from available during reserve)
		expr := "SET #resv = #resv - :qty, updated_at = :now REMOVE #order_resv.#orderID"
		cond := "attribute_exists(#order_resv.#orderID)"

		transactItems = append(transactItems, types.TransactWriteItem{
			Update: &types.Update{
				TableName:           &r.table,
				Key:                 key,
				UpdateExpression:    &expr,
				ConditionExpression: &cond,
				ExpressionAttributeNames: map[string]string{
					"#resv":       "reserved",
					"#order_resv": "order_reservations",
					"#orderID":    orderID,
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":qty": qtyAV,
					":now": nowAV,
				},
			},
		})
	}

	token := inventoryTxnToken("c", orderID)
	_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems:      transactItems,
		ClientRequestToken: &token,
	})
	if err != nil {
		return fmt.Errorf("transact confirm failed: %w", err)
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

// ListAll scans the DynamoDB table and returns all inventory items with pagination.
func (r *DynamoInventoryRepository) ListAll(ctx context.Context, limit int32, exclusiveStartKey map[string]types.AttributeValue) ([]models.Inventory, map[string]types.AttributeValue, error) {
	input := &dynamodb.ScanInput{
		TableName: &r.table,
		Limit:     &limit,
	}
	if len(exclusiveStartKey) > 0 {
		input.ExclusiveStartKey = exclusiveStartKey
	}

	out, err := r.client.Scan(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("dynamodb Scan failed: %w", err)
	}

	items := make([]models.Inventory, 0, len(out.Items))
	for _, item := range out.Items {
		var di ddbInventory
		if err := attributevalue.UnmarshalMap(item, &di); err != nil {
			return nil, nil, fmt.Errorf("unmarshal scan item: %w", err)
		}
		inv := models.Inventory{
			ProductID: di.ProductID,
			Available: di.Available,
			Reserved:  di.Reserved,
			Threshold: di.Threshold,
		}
		if t, err := time.Parse(time.RFC3339, di.UpdatedAt); err == nil {
			inv.UpdatedAt = t
		}
		items = append(items, inv)
	}

	return items, out.LastEvaluatedKey, nil
}
