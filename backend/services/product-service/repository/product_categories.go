package repository

import (
	"context"
	"fmt"
	"product-service/models"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

const productCategoryIndexName = "product-index"

// ddbProductCategory is one adjacency row: category → product.
type ddbProductCategory struct {
	CategoryID string  `dynamodbav:"category_id"`
	ProductID  string  `dynamodbav:"product_id"`
	CreatedAt  string  `dynamodbav:"created_at"`
	UpdatedAt  string  `dynamodbav:"updated_at"`
	DeletedAt  *string `dynamodbav:"deleted_at,omitempty"`
}

// PutCategoryLinks writes adjacency rows for a product's category memberships.
func (d *DynamoAdapter) PutCategoryLinks(ctx context.Context, productID uuid.UUID, categoryIDs []uuid.UUID) error {
	if d.categoryLinksTable == "" || len(categoryIDs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	pid := productID.String()
	for _, cid := range categoryIDs {
		item := ddbProductCategory{
			CategoryID: cid.String(),
			ProductID:  pid,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		av, err := attributevalue.MarshalMap(item)
		if err != nil {
			return fmt.Errorf("marshal product category link: %w", err)
		}
		if _, err := d.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: &d.categoryLinksTable,
			Item:      av,
		}); err != nil {
			return fmt.Errorf("put product category link: %w", err)
		}
	}
	return nil
}

// ReplaceCategoryLinks removes existing links for the product then writes new ones.
func (d *DynamoAdapter) ReplaceCategoryLinks(ctx context.Context, productID uuid.UUID, categoryIDs []uuid.UUID) error {
	if d.categoryLinksTable == "" {
		return nil
	}
	if err := d.DeleteCategoryLinksForProduct(ctx, productID); err != nil {
		return err
	}
	return d.PutCategoryLinks(ctx, productID, categoryIDs)
}

// DeleteCategoryLinksForProduct removes all adjacency rows for a product via product-index.
func (d *DynamoAdapter) DeleteCategoryLinksForProduct(ctx context.Context, productID uuid.UUID) error {
	if d.categoryLinksTable == "" {
		return nil
	}
	idx := productCategoryIndexName
	keyCond := "product_id = :pid"
	input := &dynamodb.QueryInput{
		TableName:              &d.categoryLinksTable,
		IndexName:              &idx,
		KeyConditionExpression: &keyCond,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pid": &types.AttributeValueMemberS{Value: productID.String()},
		},
	}
	paginator := dynamodb.NewQueryPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("query product category links: %w", err)
		}
		for _, it := range page.Items {
			var link ddbProductCategory
			if err := attributevalue.UnmarshalMap(it, &link); err != nil {
				return fmt.Errorf("unmarshal product category link: %w", err)
			}
			key, err := attributevalue.MarshalMap(map[string]string{
				"category_id": link.CategoryID,
				"product_id":  link.ProductID,
			})
			if err != nil {
				return err
			}
			if _, err := d.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
				TableName: &d.categoryLinksTable,
				Key:       key,
			}); err != nil {
				return fmt.Errorf("delete product category link: %w", err)
			}
		}
	}
	return nil
}

// ListProductIDsByCategory returns product IDs linked to any of the given categories (union).
func (d *DynamoAdapter) ListProductIDsByCategory(ctx context.Context, categoryIDs []string) ([]string, error) {
	if d.categoryLinksTable == "" || len(categoryIDs) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, catID := range categoryIDs {
		keyCond := "category_id = :cid"
		input := &dynamodb.QueryInput{
			TableName:              &d.categoryLinksTable,
			KeyConditionExpression: &keyCond,
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":cid": &types.AttributeValueMemberS{Value: catID},
			},
		}
		paginator := dynamodb.NewQueryPaginator(d.client, input)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("query category products: %w", err)
			}
			for _, it := range page.Items {
				var link ddbProductCategory
				if err := attributevalue.UnmarshalMap(it, &link); err != nil {
					return nil, err
				}
				if link.DeletedAt != nil {
					continue
				}
				if _, ok := seen[link.ProductID]; ok {
					continue
				}
				seen[link.ProductID] = struct{}{}
				out = append(out, link.ProductID)
			}
		}
	}
	return out, nil
}

// CategoryHasProducts returns true if any adjacency row exists for the category.
func (d *DynamoAdapter) CategoryHasProducts(ctx context.Context, categoryID uuid.UUID) (bool, error) {
	if d.categoryLinksTable == "" {
		return false, nil
	}
	keyCond := "category_id = :cid"
	limit := int32(1)
	out, err := d.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &d.categoryLinksTable,
		KeyConditionExpression: &keyCond,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":cid": &types.AttributeValueMemberS{Value: categoryID.String()},
		},
		Limit: &limit,
	})
	if err != nil {
		return false, fmt.Errorf("query category has products: %w", err)
	}
	return len(out.Items) > 0, nil
}

// GetProductsByIDs loads products by id (BatchGet), skipping soft-deleted.
func (d *DynamoAdapter) GetProductsByIDs(ctx context.Context, ids []string) ([]*models.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const batchSize = 100
	var results []*models.Product
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		keys := make([]map[string]types.AttributeValue, 0, end-i)
		for _, id := range ids[i:end] {
			keys = append(keys, map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: id},
			})
		}
		out, err := d.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]types.KeysAndAttributes{
				d.table: {Keys: keys},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("batch get products: %w", err)
		}
		for _, it := range out.Responses[d.table] {
			var dp ddbProduct
			if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
				return nil, err
			}
			if dp.DeletedAt != nil {
				continue
			}
			results = append(results, d.productFromDDB(&dp))
		}
	}
	return results, nil
}

func categoryIDsFromFilter(filter map[string]interface{}) ([]string, bool) {
	if filter == nil {
		return nil, false
	}
	if v, ok := filter["category_ids"].([]string); ok && len(v) > 0 {
		return v, true
	}
	if v, ok := filter["category_ids"].(string); ok && v != "" {
		return []string{v}, true
	}
	return nil, false
}

func filterWithoutCategoryIDs(filter map[string]interface{}) map[string]interface{} {
	if filter == nil {
		return nil
	}
	out := make(map[string]interface{}, len(filter))
	for k, v := range filter {
		if k == "category_ids" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func productMatchesResidualFilter(p *models.Product, filter map[string]interface{}) bool {
	if filter == nil {
		return true
	}
	if v, ok := filter["is_featured"].(bool); ok && p.IsFeatured != v {
		return false
	}
	if v, ok := filter["brand"].(string); ok && p.Brand != v {
		return false
	}
	if v, ok := filter["min_price"]; ok {
		switch n := v.(type) {
		case float64:
			if p.Price < n {
				return false
			}
		case int:
			if p.Price < float64(n) {
				return false
			}
		}
	}
	if v, ok := filter["max_price"]; ok {
		switch n := v.(type) {
		case float64:
			if p.Price > n {
				return false
			}
		case int:
			if p.Price > float64(n) {
				return false
			}
		}
	}
	if v, ok := filter["in_stock"].(bool); ok {
		if v && p.Quantity <= 0 {
			return false
		}
		if !v && p.Quantity > 0 {
			return false
		}
	}
	return true
}
