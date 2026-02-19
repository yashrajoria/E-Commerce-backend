package repository

import (
	"context"
	"errors"
	"fmt"
	"product-service/models"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// DynamoAdapter is a starter DynamoDB-backed ProductRepo implementation.
// It stores products in table with primary key `product_id` (string).
type DynamoAdapter struct {
	client *dynamodb.Client
	table  string
}

func NewDynamoAdapter(client *dynamodb.Client, table string) *DynamoAdapter {
	return &DynamoAdapter{client: client, table: table}
}

type ddbProduct struct {
	ProductID    string   `dynamodbav:"product_id"`
	Name         string   `dynamodbav:"name"`
	Price        float64  `dynamodbav:"price"`
	Quantity     int      `dynamodbav:"quantity"`
	Description  *string  `dynamodbav:"description,omitempty"`
	Images       []string `dynamodbav:"images,omitempty"`
	Brand        *string  `dynamodbav:"brand,omitempty"`
	SKU          string   `dynamodbav:"sku"`
	CategoryIDs  []string `dynamodbav:"category_ids,omitempty"`
	CategoryPath []string `dynamodbav:"category_path,omitempty"`
	IsFeatured   bool     `dynamodbav:"is_featured"`
	CreatedAt    string   `dynamodbav:"created_at"`
	UpdatedAt    string   `dynamodbav:"updated_at"`
	DeletedAt    *string  `dynamodbav:"deleted_at,omitempty"`
}

func (d *DynamoAdapter) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": id.String()})
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	out, err := d.client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &d.table, Key: key})
	if err != nil {
		return nil, fmt.Errorf("dynamodb GetItem failed: %w", err)
	}
	if len(out.Item) == 0 {
		return nil, errors.New("record not found")
	}
	var dp ddbProduct
	if err := attributevalue.UnmarshalMap(out.Item, &dp); err != nil {
		return nil, fmt.Errorf("unmarshal item: %w", err)
	}
	// Map to models.Product
	p := &models.Product{}
	p.ID, _ = uuid.Parse(dp.ProductID)
	p.Name = dp.Name
	p.Price = dp.Price
	p.Quantity = dp.Quantity
	if dp.Description != nil {
		p.Description = *dp.Description
	}
	p.Images = dp.Images
	if dp.Brand != nil {
		p.Brand = *dp.Brand
	}
	p.SKU = dp.SKU
	// convert category ids
	for _, s := range dp.CategoryIDs {
		if u, err := uuid.Parse(s); err == nil {
			p.CategoryIDs = append(p.CategoryIDs, u)
		}
	}
	p.CategoryPath = dp.CategoryPath
	p.IsFeatured = dp.IsFeatured
	if t, err := time.Parse(time.RFC3339, dp.CreatedAt); err == nil {
		p.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, dp.UpdatedAt); err == nil {
		p.UpdatedAt = t
	}
	if dp.DeletedAt != nil {
		if t, err := time.Parse(time.RFC3339, *dp.DeletedAt); err == nil {
			p.DeletedAt = &t
		}
	}
	return p, nil
}

func (d *DynamoAdapter) Create(ctx context.Context, product *models.Product) error {
	dp := ddbProduct{
		ProductID:    product.ID.String(),
		Name:         product.Name,
		Price:        product.Price,
		Quantity:     product.Quantity,
		Images:       product.Images,
		SKU:          product.SKU,
		CategoryPath: product.CategoryPath,
		IsFeatured:   product.IsFeatured,
		CreatedAt:    product.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    product.UpdatedAt.Format(time.RFC3339),
	}
	if product.DeletedAt != nil {
		s := product.DeletedAt.Format(time.RFC3339)
		dp.DeletedAt = &s
	}
	if product.Description != "" {
		dp.Description = &product.Description
	}
	if product.Brand != "" {
		dp.Brand = &product.Brand
	}
	for _, uid := range product.CategoryIDs {
		dp.CategoryIDs = append(dp.CategoryIDs, uid.String())
	}

	item, err := attributevalue.MarshalMap(dp)
	if err != nil {
		return fmt.Errorf("marshal product: %w", err)
	}
	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{TableName: &d.table, Item: item})
	if err != nil {
		return fmt.Errorf("dynamodb PutItem failed: %w", err)
	}
	return nil
}

// Find performs a Scan with basic pagination. Filter support is limited to nil (no filter).
func (d *DynamoAdapter) Find(ctx context.Context, filter map[string]interface{}, limit, skip int) ([]*models.Product, error) {
	// Simple implementation: Scan table and apply skip/limit
	input := &dynamodb.ScanInput{TableName: &d.table}
	var results []*models.Product
	paginator := dynamodb.NewScanPaginator(d.client, input)
	seen := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan page failed: %w", err)
		}
		for _, it := range page.Items {
			if skip > 0 && seen < skip {
				seen++
				continue
			}
			var dp ddbProduct
			if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
				return nil, fmt.Errorf("unmarshal item: %w", err)
			}
			p := &models.Product{}
			p.ID, _ = uuid.Parse(dp.ProductID)
			p.Name = dp.Name
			p.Price = dp.Price
			p.Quantity = dp.Quantity
			if dp.Description != nil {
				p.Description = *dp.Description
			}
			p.Images = dp.Images
			if dp.Brand != nil {
				p.Brand = *dp.Brand
			}
			p.SKU = dp.SKU
			for _, s := range dp.CategoryIDs {
				if u, err := uuid.Parse(s); err == nil {
					p.CategoryIDs = append(p.CategoryIDs, u)
				}
			}
			p.CategoryPath = dp.CategoryPath
			p.IsFeatured = dp.IsFeatured
			if t, err := time.Parse(time.RFC3339, dp.CreatedAt); err == nil {
				p.CreatedAt = t
			}
			if t, err := time.Parse(time.RFC3339, dp.UpdatedAt); err == nil {
				p.UpdatedAt = t
			}
			if dp.DeletedAt != nil {
				if t, err := time.Parse(time.RFC3339, *dp.DeletedAt); err == nil {
					p.DeletedAt = &t
				}
			}
			results = append(results, p)
			if limit > 0 && len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

// Count returns the item count (full table scan Count)
func (d *DynamoAdapter) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	input := &dynamodb.ScanInput{TableName: &d.table, Select: types.SelectCount}
	paginator := dynamodb.NewScanPaginator(d.client, input)
	var total int64
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("scan count failed: %w", err)
		}
		total += int64(page.Count)
	}
	return total, nil
}

// CreateMany uses BatchWriteItem (chunks of 25)
func (d *DynamoAdapter) CreateMany(ctx context.Context, products []models.Product) error {
	const chunkSize = 25
	for i := 0; i < len(products); i += chunkSize {
		end := i + chunkSize
		if end > len(products) {
			end = len(products)
		}
		writeReqs := make([]types.WriteRequest, 0, end-i)
		for _, p := range products[i:end] {
			dp := ddbProduct{
				ProductID:    p.ID.String(),
				Name:         p.Name,
				Price:        p.Price,
				Quantity:     p.Quantity,
				Images:       p.Images,
				SKU:          p.SKU,
				CategoryPath: p.CategoryPath,
				IsFeatured:   p.IsFeatured,
				CreatedAt:    p.CreatedAt.Format(time.RFC3339),
				UpdatedAt:    p.UpdatedAt.Format(time.RFC3339),
			}
			if p.Description != "" {
				dp.Description = &p.Description
			}
			if p.Brand != "" {
				dp.Brand = &p.Brand
			}
			for _, uid := range p.CategoryIDs {
				dp.CategoryIDs = append(dp.CategoryIDs, uid.String())
			}
			item, err := attributevalue.MarshalMap(dp)
			if err != nil {
				return fmt.Errorf("marshal batch item: %w", err)
			}
			writeReqs = append(writeReqs, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}})
		}
		req := &dynamodb.BatchWriteItemInput{RequestItems: map[string][]types.WriteRequest{d.table: writeReqs}}
		// Retry unprocessed items with exponential backoff (simple strategy)
		attempts := 0
		for {
			out, err := d.client.BatchWriteItem(ctx, req)
			if err != nil {
				return fmt.Errorf("batch write failed: %w", err)
			}
			// If there are no unprocessed items, we're done for this chunk
			if len(out.UnprocessedItems) == 0 {
				break
			}
			// Prepare to retry only the unprocessed items for this table
			if unp, ok := out.UnprocessedItems[d.table]; ok && len(unp) > 0 {
				req.RequestItems[d.table] = unp
			} else {
				break
			}
			attempts++
			if attempts >= 3 {
				return fmt.Errorf("batch write had unprocessed items after retries")
			}
			// simple backoff
			time.Sleep(time.Duration(attempts*300) * time.Millisecond)
		}
	}
	return nil
}

// Update performs UpdateItem by setting provided attributes
func (d *DynamoAdapter) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	expr := "SET "
	exprVals := make(map[string]types.AttributeValue)
	i := 0
	for k, v := range updates {
		ph := fmt.Sprintf(":v%d", i)
		if i > 0 {
			expr += ", "
		}
		expr += fmt.Sprintf("%s = %s", k, ph)
		av, err := attributevalue.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshal update value: %w", err)
		}
		exprVals[ph] = av
		i++
	}
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": id.String()})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	// convert exprVals to map[string]types.AttributeValue
	avMap := make(map[string]types.AttributeValue)
	for k, v := range exprVals {
		avMap[k] = v
	}
	_, err = d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{TableName: &d.table, Key: key, UpdateExpression: &expr, ExpressionAttributeValues: avMap})
	if err != nil {
		return fmt.Errorf("update item failed: %w", err)
	}
	return nil
}

func (d *DynamoAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	key, err := attributevalue.MarshalMap(map[string]string{"product_id": id.String()})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	_, err = d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: &d.table, Key: key})
	if err != nil {
		return fmt.Errorf("delete item failed: %w", err)
	}
	return nil
}

func (d *DynamoAdapter) FindBySKUs(ctx context.Context, skus []string) ([]models.Product, error) {
	if len(skus) == 0 {
		return nil, nil
	}
	// Build FilterExpression: sku IN (:s0, :s1...)
	expr := ""
	values := make(map[string]types.AttributeValue)
	for i, s := range skus {
		ph := fmt.Sprintf(":s%d", i)
		if i > 0 {
			expr += ", "
		}
		expr += ph
		av, err := attributevalue.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("marshal sku: %w", err)
		}
		values[ph] = av
	}
	filterExpr := fmt.Sprintf("sku IN (%s)", expr)
	input := &dynamodb.ScanInput{TableName: &d.table, FilterExpression: &filterExpr, ExpressionAttributeValues: values}
	out, err := d.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scan for skus failed: %w", err)
	}
	var res []models.Product
	for _, it := range out.Items {
		var dp ddbProduct
		if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
			return nil, fmt.Errorf("unmarshal item: %w", err)
		}
		p := models.Product{}
		p.ID, _ = uuid.Parse(dp.ProductID)
		p.Name = dp.Name
		p.Price = dp.Price
		p.Quantity = dp.Quantity
		if dp.Description != nil {
			p.Description = *dp.Description
		}
		p.Images = dp.Images
		if dp.Brand != nil {
			p.Brand = *dp.Brand
		}
		p.SKU = dp.SKU
		for _, s := range dp.CategoryIDs {
			if u, err := uuid.Parse(s); err == nil {
				p.CategoryIDs = append(p.CategoryIDs, u)
			}
		}
		p.CategoryPath = dp.CategoryPath
		p.IsFeatured = dp.IsFeatured
		if t, err := time.Parse(time.RFC3339, dp.CreatedAt); err == nil {
			p.CreatedAt = t
		}
		if t, err := time.Parse(time.RFC3339, dp.UpdatedAt); err == nil {
			p.UpdatedAt = t
		}
		if dp.DeletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dp.DeletedAt); err == nil {
				p.DeletedAt = &t
			}
		}
		res = append(res, p)
	}
	return res, nil
}

func (d *DynamoAdapter) EnsureIndexes(ctx context.Context) error {
	// Dynamo table / GSI creation should be handled by infrastructure init or IaC.
	return nil
}
