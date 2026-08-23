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

// DynamoAdapter is a DynamoDB-backed ProductRepo implementation.
// It stores products in table with primary key `id` (string).
type DynamoAdapter struct {
	client             *dynamodb.Client
	table              string
	categoryLinksTable string
}

func NewDynamoAdapter(client *dynamodb.Client, table string) *DynamoAdapter {
	return &DynamoAdapter{client: client, table: table}
}

// WithCategoryLinksTable enables ProductCategories adjacency for category→product Query.
func (d *DynamoAdapter) WithCategoryLinksTable(table string) *DynamoAdapter {
	d.categoryLinksTable = table
	return d
}

const (
	skuIndexName      = "sku-index"
	featuredIndexName = "featured-index"
	brandIndexName    = "brand-index"
)

type ddbProduct struct {
	ProductID    string   `dynamodbav:"id"`
	Name         string   `dynamodbav:"name"`
	Price        float64  `dynamodbav:"price"`
	Quantity     int      `dynamodbav:"quantity"`
	Description  *string  `dynamodbav:"description,omitempty"`
	Images       []string `dynamodbav:"images,omitempty"`
	Brand        *string  `dynamodbav:"brand,omitempty"`
	SKU          string   `dynamodbav:"sku"`
	CategoryIDs  []string `dynamodbav:"category_ids,omitempty"`
	CategoryPath []string `dynamodbav:"category_path,omitempty"`
	// IsFeatured is stored as "true"/"false" string for the featured-index GSI HASH key.
	IsFeatured string  `dynamodbav:"is_featured"`
	CreatedAt  string  `dynamodbav:"created_at"`
	UpdatedAt  string  `dynamodbav:"updated_at"`
	DeletedAt  *string `dynamodbav:"deleted_at,omitempty"`
}

func featuredKey(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func parseFeatured(s string) bool {
	return strings.EqualFold(s, "true") || s == "1"
}

func (d *DynamoAdapter) productFromDDB(dp *ddbProduct) *models.Product {
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
	p.IsFeatured = parseFeatured(dp.IsFeatured)
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
	return p
}

func (d *DynamoAdapter) toDDB(product *models.Product) ddbProduct {
	dp := ddbProduct{
		ProductID:    product.ID.String(),
		Name:         product.Name,
		Price:        product.Price,
		Quantity:     product.Quantity,
		Images:       product.Images,
		SKU:          product.SKU,
		CategoryPath: product.CategoryPath,
		IsFeatured:   featuredKey(product.IsFeatured),
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
	return dp
}

func (d *DynamoAdapter) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
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
	var dp ddbProduct
	if err := attributevalue.UnmarshalMap(out.Item, &dp); err != nil {
		return nil, fmt.Errorf("unmarshal item: %w", err)
	}
	return d.productFromDDB(&dp), nil
}

func (d *DynamoAdapter) Create(ctx context.Context, product *models.Product) error {
	dp := d.toDDB(product)

	item, err := attributevalue.MarshalMap(dp)
	if err != nil {
		return fmt.Errorf("marshal product: %w", err)
	}
	_, err = d.client.PutItem(ctx, &dynamodb.PutItemInput{TableName: &d.table, Item: item})
	if err != nil {
		return fmt.Errorf("dynamodb PutItem failed: %w", err)
	}
	if err := d.PutCategoryLinks(ctx, product.ID, product.CategoryIDs); err != nil {
		return fmt.Errorf("sync category links: %w", err)
	}
	return nil
}

// buildFilterExpression constructs DynamoDB filter expressions from the map.
// Product reads exclude soft-deleted records by default.
func (d *DynamoAdapter) buildFilterExpression(filter map[string]interface{}) (filterExpr *string, attrValues map[string]types.AttributeValue, attrNames map[string]string) {
	var expressions []string
	attrValues = make(map[string]types.AttributeValue)
	attrNames = make(map[string]string)

	attrNames["#deleted_at"] = "deleted_at"
	expressions = append(expressions, "attribute_not_exists(#deleted_at)")

	if filter == nil {
		joined := strings.Join(expressions, " AND ")
		return &joined, nil, attrNames
	}

	i := 0

	if v, ok := filter["is_featured"].(bool); ok {
		ph := fmt.Sprintf(":v%d", i)
		nm := fmt.Sprintf("#f%d", i)
		expressions = append(expressions, fmt.Sprintf("%s = %s", nm, ph))
		av, _ := attributevalue.Marshal(featuredKey(v))
		attrValues[ph] = av
		attrNames[nm] = "is_featured"
		i++
	}

	if v, ok := filter["brand"].(string); ok {
		ph := fmt.Sprintf(":v%d", i)
		nm := fmt.Sprintf("#f%d", i)
		expressions = append(expressions, fmt.Sprintf("%s = %s", nm, ph))
		av, _ := attributevalue.Marshal(v)
		attrValues[ph] = av
		attrNames[nm] = "brand"
		i++
	}

	if v, ok := filter["min_price"]; ok {
		ph := fmt.Sprintf(":v%d", i)
		nm := fmt.Sprintf("#f%d", i)
		expressions = append(expressions, fmt.Sprintf("%s >= %s", nm, ph))
		av, _ := attributevalue.Marshal(v)
		attrValues[ph] = av
		attrNames[nm] = "price"
		i++
	}

	if v, ok := filter["max_price"]; ok {
		ph := fmt.Sprintf(":v%d", i)
		nm := fmt.Sprintf("#f%d", i)
		expressions = append(expressions, fmt.Sprintf("%s <= %s", nm, ph))
		av, _ := attributevalue.Marshal(v)
		attrValues[ph] = av
		attrNames[nm] = "price"
		i++
	}

	if v, ok := filter["category_ids"].([]string); ok && len(v) > 0 {
		var catExprs []string
		nm := fmt.Sprintf("#f%d", i)
		attrNames[nm] = "category_ids"
		for j, catID := range v {
			ph := fmt.Sprintf(":cat%d", j)
			catExprs = append(catExprs, fmt.Sprintf("contains(%s, %s)", nm, ph))
			av, _ := attributevalue.Marshal(catID)
			attrValues[ph] = av
		}
		expressions = append(expressions, "("+strings.Join(catExprs, " OR ")+")")
		i++
	}

	if v, ok := filter["in_stock"].(bool); ok {
		nm := fmt.Sprintf("#f%d", i)
		attrNames[nm] = "quantity"
		if v {
			expressions = append(expressions, fmt.Sprintf("%s > :zero", nm))
		} else {
			expressions = append(expressions, fmt.Sprintf("%s <= :zero", nm))
		}
		if _, exists := attrValues[":zero"]; !exists {
			av, _ := attributevalue.Marshal(0)
			attrValues[":zero"] = av
		}
		i++
	}

	joined := strings.Join(expressions, " AND ")
	if len(attrValues) == 0 {
		attrValues = nil
	}
	return &joined, attrValues, attrNames
}

// isFeaturedOnlyFilter is true when the only business filter is is_featured
// (soft-delete exclusion is always applied). Suitable for featured-index Query.
func isFeaturedOnlyFilter(filter map[string]interface{}) (bool, bool) {
	if filter == nil {
		return false, false
	}
	featured, hasFeatured := filter["is_featured"].(bool)
	if !hasFeatured {
		return false, false
	}
	for k := range filter {
		if k != "is_featured" {
			return false, false
		}
	}
	return true, featured
}

// brandFromFilter extracts a brand filter value, if present.
func brandFromFilter(filter map[string]interface{}) (string, bool) {
	if filter == nil {
		return "", false
	}
	v, ok := filter["brand"].(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func filterWithoutBrand(filter map[string]interface{}) map[string]interface{} {
	if filter == nil {
		return nil
	}
	out := make(map[string]interface{}, len(filter))
	for k, v := range filter {
		if k == "brand" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Find uses featured-index Query when filtering only by is_featured;
// ProductCategories Query when filtering by category_ids; brand-index Query
// when filtering by brand (with or without price/stock/featured refinements);
// otherwise falls back to a full table Scan.
func (d *DynamoAdapter) Find(ctx context.Context, filter map[string]interface{}, limit, skip int) ([]*models.Product, error) {
	if ok, featured := isFeaturedOnlyFilter(filter); ok {
		return d.findFeatured(ctx, featured, limit, skip)
	}
	if catIDs, ok := categoryIDsFromFilter(filter); ok && d.categoryLinksTable != "" {
		return d.findByCategories(ctx, catIDs, filterWithoutCategoryIDs(filter), limit, skip)
	}
	if brand, ok := brandFromFilter(filter); ok {
		return d.findByBrand(ctx, brand, filterWithoutBrand(filter), limit, skip)
	}

	filterExpr, attrValues, attrNames := d.buildFilterExpression(filter)

	input := &dynamodb.ScanInput{
		TableName:                 &d.table,
		FilterExpression:          filterExpr,
		ExpressionAttributeValues: attrValues,
		ExpressionAttributeNames:  attrNames,
	}

	var results []*models.Product
	paginator := dynamodb.NewScanPaginator(d.client, input)
	seenMatches := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan page failed: %w", err)
		}
		for _, it := range page.Items {
			if skip > 0 && seenMatches < skip {
				seenMatches++
				continue
			}

			var dp ddbProduct
			if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
				return nil, fmt.Errorf("unmarshal item: %w", err)
			}
			results = append(results, d.productFromDDB(&dp))
			if limit > 0 && len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

func (d *DynamoAdapter) findByCategories(ctx context.Context, catIDs []string, residual map[string]interface{}, limit, skip int) ([]*models.Product, error) {
	ids, err := d.ListProductIDsByCategory(ctx, catIDs)
	if err != nil {
		return nil, err
	}
	products, err := d.GetProductsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	var results []*models.Product
	seenMatches := 0
	for _, p := range products {
		if !productMatchesResidualFilter(p, residual) {
			continue
		}
		if skip > 0 && seenMatches < skip {
			seenMatches++
			continue
		}
		results = append(results, p)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

// findByBrand queries the brand-index GSI instead of scanning the whole table,
// then applies any residual filters (price range, is_featured, in_stock) in memory.
func (d *DynamoAdapter) findByBrand(ctx context.Context, brand string, residual map[string]interface{}, limit, skip int) ([]*models.Product, error) {
	idx := brandIndexName
	keyCond := "#brand = :brand"
	filterExpr := "attribute_not_exists(#deleted_at)"
	input := &dynamodb.QueryInput{
		TableName:              &d.table,
		IndexName:              &idx,
		KeyConditionExpression: &keyCond,
		FilterExpression:       &filterExpr,
		ExpressionAttributeNames: map[string]string{
			"#brand":      "brand",
			"#deleted_at": "deleted_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":brand": &types.AttributeValueMemberS{Value: brand},
		},
		ScanIndexForward: ptrBool(false), // newest created_at first
	}

	var results []*models.Product
	paginator := dynamodb.NewQueryPaginator(d.client, input)
	seenMatches := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("brand query failed: %w", err)
		}
		for _, it := range page.Items {
			var dp ddbProduct
			if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
				return nil, fmt.Errorf("unmarshal item: %w", err)
			}
			p := d.productFromDDB(&dp)
			if !productMatchesResidualFilter(p, residual) {
				continue
			}
			if skip > 0 && seenMatches < skip {
				seenMatches++
				continue
			}
			results = append(results, p)
			if limit > 0 && len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

func (d *DynamoAdapter) findFeatured(ctx context.Context, featured bool, limit, skip int) ([]*models.Product, error) {
	idx := featuredIndexName
	keyCond := "#feat = :feat"
	filterExpr := "attribute_not_exists(#deleted_at)"
	input := &dynamodb.QueryInput{
		TableName:              &d.table,
		IndexName:              &idx,
		KeyConditionExpression: &keyCond,
		FilterExpression:       &filterExpr,
		ExpressionAttributeNames: map[string]string{
			"#feat":       "is_featured",
			"#deleted_at": "deleted_at",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":feat": &types.AttributeValueMemberS{Value: featuredKey(featured)},
		},
		ScanIndexForward: ptrBool(false), // newest created_at first
	}

	var results []*models.Product
	paginator := dynamodb.NewQueryPaginator(d.client, input)
	seenMatches := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("featured query failed: %w", err)
		}
		for _, it := range page.Items {
			if skip > 0 && seenMatches < skip {
				seenMatches++
				continue
			}
			var dp ddbProduct
			if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
				return nil, fmt.Errorf("unmarshal item: %w", err)
			}
			results = append(results, d.productFromDDB(&dp))
			if limit > 0 && len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

func ptrBool(v bool) *bool { return &v }

// Count returns the item count using FilterExpression if provided
func (d *DynamoAdapter) Count(ctx context.Context, filter map[string]interface{}) (int64, error) {
	if catIDs, ok := categoryIDsFromFilter(filter); ok && d.categoryLinksTable != "" {
		ids, err := d.ListProductIDsByCategory(ctx, catIDs)
		if err != nil {
			return 0, err
		}
		residual := filterWithoutCategoryIDs(filter)
		if residual == nil {
			return int64(len(ids)), nil
		}
		products, err := d.GetProductsByIDs(ctx, ids)
		if err != nil {
			return 0, err
		}
		var n int64
		for _, p := range products {
			if productMatchesResidualFilter(p, residual) {
				n++
			}
		}
		return n, nil
	}
	if brand, ok := brandFromFilter(filter); ok {
		products, err := d.findByBrand(ctx, brand, filterWithoutBrand(filter), 0, 0)
		if err != nil {
			return 0, err
		}
		return int64(len(products)), nil
	}

	filterExpr, attrValues, attrNames := d.buildFilterExpression(filter)

	input := &dynamodb.ScanInput{
		TableName:                 &d.table,
		FilterExpression:          filterExpr,
		ExpressionAttributeValues: attrValues,
		ExpressionAttributeNames:  attrNames,
		Select:                    types.SelectCount,
	}

	var total int64
	paginator := dynamodb.NewScanPaginator(d.client, input)
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
			prod := p
			dp := d.toDDB(&prod)
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
		for _, p := range products[i:end] {
			if err := d.PutCategoryLinks(ctx, p.ID, p.CategoryIDs); err != nil {
				return fmt.Errorf("sync category links for %s: %w", p.ID, err)
			}
		}
	}
	return nil
}

// Update performs UpdateItem by setting whitelisted attributes and refusing to
// create missing/deleted records implicitly.
func (d *DynamoAdapter) Update(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	allowed := map[string]bool{
		"name": true, "price": true, "quantity": true, "description": true,
		"images": true, "brand": true, "sku": true, "category_ids": true,
		"category_path": true, "is_featured": true,
	}

	expr := "SET "
	exprVals := make(map[string]types.AttributeValue)
	exprNames := make(map[string]string)
	i := 0
	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		ph := fmt.Sprintf(":v%d", i)
		namePh := fmt.Sprintf("#f%d", i)
		if i > 0 {
			expr += ", "
		}
		expr += fmt.Sprintf("%s = %s", namePh, ph)
		exprNames[namePh] = k
		val := v
		if k == "is_featured" {
			if b, ok := v.(bool); ok {
				val = featuredKey(b)
			}
		}
		av, err := attributevalue.Marshal(val)
		if err != nil {
			return fmt.Errorf("marshal update value: %w", err)
		}
		exprVals[ph] = av
		i++
	}
	if i == 0 {
		return nil
	}

	updatedAtAV, err := attributevalue.Marshal(time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("marshal updated_at: %w", err)
	}
	expr += ", #updated_at = :updated_at"
	exprNames["#updated_at"] = "updated_at"
	exprNames["#deleted_at"] = "deleted_at"
	exprVals[":updated_at"] = updatedAtAV

	key, err := attributevalue.MarshalMap(map[string]string{"id": id.String()})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	cond := "attribute_exists(id) AND attribute_not_exists(#deleted_at)"
	_, err = d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.table,
		Key:                       key,
		UpdateExpression:          &expr,
		ExpressionAttributeValues: exprVals,
		ExpressionAttributeNames:  exprNames,
		ConditionExpression:       &cond,
	})
	if err != nil {
		return fmt.Errorf("update item failed: %w", err)
	}
	if raw, ok := updates["category_ids"]; ok {
		var cats []uuid.UUID
		switch v := raw.(type) {
		case []uuid.UUID:
			cats = v
		case []string:
			for _, s := range v {
				if u, err := uuid.Parse(s); err == nil {
					cats = append(cats, u)
				}
			}
		}
		if err := d.ReplaceCategoryLinks(ctx, id, cats); err != nil {
			return fmt.Errorf("sync category links: %w", err)
		}
	}
	return nil
}

func (d *DynamoAdapter) Delete(ctx context.Context, id uuid.UUID) error {
	key, err := attributevalue.MarshalMap(map[string]string{"id": id.String()})
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	deletedAtAV, err := attributevalue.Marshal(now)
	if err != nil {
		return fmt.Errorf("marshal deleted_at: %w", err)
	}
	expr := "SET #deleted_at = :deleted_at, #updated_at = :deleted_at"
	cond := "attribute_exists(id) AND attribute_not_exists(#deleted_at)"
	_, err = d.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.table,
		Key:                       key,
		UpdateExpression:          &expr,
		ExpressionAttributeValues: map[string]types.AttributeValue{":deleted_at": deletedAtAV},
		ExpressionAttributeNames: map[string]string{
			"#deleted_at": "deleted_at",
			"#updated_at": "updated_at",
		},
		ConditionExpression: &cond,
	})
	if err != nil {
		return fmt.Errorf("soft delete item failed: %w", err)
	}
	_ = d.DeleteCategoryLinksForProduct(ctx, id)
	return nil
}

// DeleteMany soft-deletes multiple products.
func (d *DynamoAdapter) DeleteMany(ctx context.Context, ids []uuid.UUID) error {
	for _, id := range ids {
		if err := d.Delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (d *DynamoAdapter) FindBySKUs(ctx context.Context, skus []string) ([]models.Product, error) {
	if len(skus) == 0 {
		return nil, nil
	}
	idx := skuIndexName
	filterExpr := "attribute_not_exists(#deleted_at)"
	exprNames := map[string]string{"#deleted_at": "deleted_at", "#sku": "sku"}
	var res []models.Product
	seen := make(map[string]struct{})

	for _, sku := range skus {
		if sku == "" {
			continue
		}
		if _, dup := seen[sku]; dup {
			continue
		}
		seen[sku] = struct{}{}

		keyCond := "#sku = :sku"
		input := &dynamodb.QueryInput{
			TableName:              &d.table,
			IndexName:              &idx,
			KeyConditionExpression: &keyCond,
			FilterExpression:       &filterExpr,
			ExpressionAttributeNames: exprNames,
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":sku": &types.AttributeValueMemberS{Value: sku},
			},
		}
		paginator := dynamodb.NewQueryPaginator(d.client, input)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("sku query failed: %w", err)
			}
			for _, it := range page.Items {
				var dp ddbProduct
				if err := attributevalue.UnmarshalMap(it, &dp); err != nil {
					return nil, fmt.Errorf("unmarshal item: %w", err)
				}
				res = append(res, *d.productFromDDB(&dp))
			}
		}
	}
	return res, nil
}

func (d *DynamoAdapter) EnsureIndexes(ctx context.Context) error {
	// Dynamo table / GSI creation should be handled by infrastructure init or IaC.
	return nil
}
