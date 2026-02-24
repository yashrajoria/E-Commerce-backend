package repository

import (
	"context"
	"errors"
	"fmt"
	"product-service/models"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

// DynamoCategoryAdapter is a DynamoDB-backed CategoryRepo implementation.
type DynamoCategoryAdapter struct {
	client       *dynamodb.Client
	table        string
	productTable string
}

func NewDynamoCategoryAdapter(client *dynamodb.Client, table, productTable string) *DynamoCategoryAdapter {
	return &DynamoCategoryAdapter{client: client, table: table, productTable: productTable}
}

type ddbCategory struct {
	CategoryID string   `dynamodbav:"id"`
	Name       string   `dynamodbav:"name"`
	ParentIDs  []string `dynamodbav:"parent_ids,omitempty"`
	Image      string   `dynamodbav:"image,omitempty"`
	Ancestors  []string `dynamodbav:"ancestors,omitempty"`
	Slug       string   `dynamodbav:"slug"`
	Path       []string `dynamodbav:"path,omitempty"`
	Level      int      `dynamodbav:"level,omitempty"`
	IsActive   bool     `dynamodbav:"is_active"`
	CreatedAt  string   `dynamodbav:"created_at"`
	UpdatedAt  string   `dynamodbav:"updated_at"`
	DeletedAt  *string  `dynamodbav:"deleted_at,omitempty"`
}

func (d *DynamoCategoryAdapter) toModel(dc *ddbCategory) *models.Category {
	cat := &models.Category{}
	cat.ID, _ = uuid.Parse(dc.CategoryID)
	cat.Name = dc.Name
	cat.Image = dc.Image
	cat.Slug = dc.Slug
	cat.Path = dc.Path
	cat.Level = dc.Level
	cat.IsActive = dc.IsActive

	for _, s := range dc.ParentIDs {
		if u, err := uuid.Parse(s); err == nil {
			cat.ParentIDs = append(cat.ParentIDs, u)
		}
	}
	for _, s := range dc.Ancestors {
		if u, err := uuid.Parse(s); err == nil {
			cat.Ancestors = append(cat.Ancestors, u)
		}
	}
	if t, err := time.Parse(time.RFC3339, dc.CreatedAt); err == nil {
		cat.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, dc.UpdatedAt); err == nil {
		cat.UpdatedAt = t
	}
	if dc.DeletedAt != nil {
		if t, err := time.Parse(time.RFC3339, *dc.DeletedAt); err == nil {
			cat.DeletedAt = &t
		}
	}
	return cat
}

func (d *DynamoCategoryAdapter) toDDB(cat *models.Category) *ddbCategory {
	dc := &ddbCategory{
		CategoryID: cat.ID.String(),
		Name:       cat.Name,
		Image:      cat.Image,
		Slug:       cat.Slug,
		Path:       cat.Path,
		Level:      cat.Level,
		IsActive:   cat.IsActive,
		CreatedAt:  cat.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  cat.UpdatedAt.Format(time.RFC3339),
	}
	for _, uid := range cat.ParentIDs {
		dc.ParentIDs = append(dc.ParentIDs, uid.String())
	}
	for _, uid := range cat.Ancestors {
		dc.Ancestors = append(dc.Ancestors, uid.String())
	}
	if cat.DeletedAt != nil {
		s := cat.DeletedAt.Format(time.RFC3339)
		dc.DeletedAt = &s
	}
	return dc
}

func (d *DynamoCategoryAdapter) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	key, err := attributevalue.MarshalMap(map[string]string{"id": id.String()})
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
	var dc ddbCategory
	if err := attributevalue.UnmarshalMap(out.Item, &dc); err != nil {
		return nil, fmt.Errorf("unmarshal item: %w", err)
	}
	// Skip soft-deleted
	if dc.DeletedAt != nil {
		return nil, errors.New("record not found")
	}
	return d.toModel(&dc), nil
}

func (d *DynamoCategoryAdapter) FindByName(ctx context.Context, name string) (*models.Category, error) {
	// Scan with filter (for production, use GSI on name)
	filterExpr := "attribute_not_exists(deleted_at) AND #n = :name"
	exprNames := map[string]string{"#n": "name"}
	exprVals, _ := attributevalue.MarshalMap(map[string]string{":name": name})

	input := &dynamodb.ScanInput{
		TableName:                 &d.table,
		FilterExpression:          &filterExpr,
		ExpressionAttributeNames:  exprNames,
		ExpressionAttributeValues: exprVals,
	}
	out, err := d.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	if len(out.Items) == 0 {
		return nil, errors.New("record not found")
	}
	var dc ddbCategory
	if err := attributevalue.UnmarshalMap(out.Items[0], &dc); err != nil {
		return nil, fmt.Errorf("unmarshal item: %w", err)
	}
	return d.toModel(&dc), nil
}

func (d *DynamoCategoryAdapter) FindByNames(ctx context.Context, names []string) ([]models.Category, error) {
	if len(names) == 0 {
		return []models.Category{}, nil
	}

	// Build filter: name IN (:n0, :n1, ...)
	placeholders := make([]string, len(names))
	exprVals := make(map[string]types.AttributeValue)
	for i, name := range names {
		ph := fmt.Sprintf(":n%d", i)
		placeholders[i] = ph
		av, _ := attributevalue.Marshal(name)
		exprVals[ph] = av
	}
	filterExpr := fmt.Sprintf("attribute_not_exists(deleted_at) AND #n IN (%s)", strings.Join(placeholders, ", "))
	exprNames := map[string]string{"#n": "name"}

	input := &dynamodb.ScanInput{
		TableName:                 &d.table,
		FilterExpression:          &filterExpr,
		ExpressionAttributeNames:  exprNames,
		ExpressionAttributeValues: exprVals,
	}

	out, err := d.client.Scan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	var results []models.Category
	for _, item := range out.Items {
		var dc ddbCategory
		if err := attributevalue.UnmarshalMap(item, &dc); err != nil {
			continue
		}
		results = append(results, *d.toModel(&dc))
	}
	return results, nil
}

func (d *DynamoCategoryAdapter) FindAll(ctx context.Context) ([]models.Category, error) {
	filterExpr := "attribute_not_exists(deleted_at)"
	input := &dynamodb.ScanInput{
		TableName:        &d.table,
		FilterExpression: &filterExpr,
	}

	var results []models.Category
	paginator := dynamodb.NewScanPaginator(d.client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan page failed: %w", err)
		}
		for _, item := range page.Items {
			var dc ddbCategory
			if err := attributevalue.UnmarshalMap(item, &dc); err != nil {
				continue
			}
			results = append(results, *d.toModel(&dc))
		}
	}
	return results, nil
}

func (d *DynamoCategoryAdapter) Create(ctx context.Context, category *models.Category) error {
	dc := d.toDDB(category)
	item, err := attributevalue.MarshalMap(dc)
	if err != nil {
		return fmt.Errorf("marshal category: %w", err)
	}
	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{TableName: &d.table, Item: item})
	if err != nil {
		return fmt.Errorf("dynamodb PutItem failed: %w", err)
	}
	return nil
}

func (d *DynamoCategoryAdapter) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	expr := "SET "
	exprNames := make(map[string]string)
	exprVals := make(map[string]types.AttributeValue)
	i := 0
	for k, v := range updates {
		ph := fmt.Sprintf(":v%d", i)
		attrName := fmt.Sprintf("#a%d", i)
		if i > 0 {
			expr += ", "
		}
		expr += fmt.Sprintf("%s = %s", attrName, ph)
		exprNames[attrName] = k
		av, err := attributevalue.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshal update value: %w", err)
		}
		exprVals[ph] = av
		i++
	}

	key, err := attributevalue.MarshalMap(map[string]string{"id": id.String()})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}

	_, err = d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.table,
		Key:                       key,
		UpdateExpression:          &expr,
		ExpressionAttributeNames:  exprNames,
		ExpressionAttributeValues: exprVals,
	})
	if err != nil {
		return fmt.Errorf("update item failed: %w", err)
	}
	return nil
}

func (d *DynamoCategoryAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	// Soft delete
	now := time.Now().UTC().Format(time.RFC3339)
	return d.Update(ctx, id, map[string]interface{}{
		"deleted_at": now,
		"updated_at": now,
	})
}

// HasProducts checks if any products reference this category
func (d *DynamoCategoryAdapter) HasProducts(ctx context.Context, categoryID uuid.UUID) (bool, error) {
	// Scan products table for category_ids containing this ID
	filterExpr := "attribute_not_exists(deleted_at) AND contains(category_ids, :catId)"
	exprVals, _ := attributevalue.MarshalMap(map[string]string{":catId": categoryID.String()})

	input := &dynamodb.ScanInput{
		TableName:                 &d.productTable,
		FilterExpression:          &filterExpr,
		ExpressionAttributeValues: exprVals,
		Limit:                     ptrInt32(1), // We only need to know if at least one exists
	}

	out, err := d.client.Scan(ctx, input)
	if err != nil {
		return false, fmt.Errorf("scan products failed: %w", err)
	}
	return len(out.Items) > 0, nil
}

func ptrInt32(v int32) *int32 {
	return &v
}
