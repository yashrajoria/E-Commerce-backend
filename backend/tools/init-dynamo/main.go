package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	awspkg "github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws"
)

type Product struct {
	ID           string   `dynamodbav:"id"`
	Name         string   `dynamodbav:"name"`
	Price        float64  `dynamodbav:"price"`
	Quantity     int      `dynamodbav:"quantity"`
	Description  string   `dynamodbav:"description"`
	Images       []string `dynamodbav:"images"`
	Brand        string   `dynamodbav:"brand"`
	SKU          string   `dynamodbav:"sku"`
	CategoryIDs  []string `dynamodbav:"category_ids"`
	CategoryPath []string `dynamodbav:"category_path"`
	IsFeatured   string   `dynamodbav:"is_featured"`
	CreatedAt    string   `dynamodbav:"created_at"`
	UpdatedAt    string   `dynamodbav:"updated_at"`
}

type Category struct {
	ID        string `dynamodbav:"id"`
	Name      string `dynamodbav:"name"`
	Slug      string `dynamodbav:"slug"`
	CreatedAt string `dynamodbav:"created_at"`
	UpdatedAt string `dynamodbav:"updated_at"`
}

type ProductCategory struct {
	CategoryID string `dynamodbav:"category_id"`
	ProductID  string `dynamodbav:"product_id"`
	CreatedAt  string `dynamodbav:"created_at"`
}

type Inventory struct {
	ID        string `dynamodbav:"id"`
	Available int    `dynamodbav:"available"`
	Reserved  int    `dynamodbav:"reserved"`
	UpdatedAt string `dynamodbav:"updated_at"`
}

func main() {
	ctx := context.Background()
	cfg, err := awspkg.LoadConfig(ctx)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	// Wait up to 30 seconds for DynamoDB to be ready
	log.Println("Checking DynamoDB availability...")
	for i := 0; i < 15; i++ {
		_, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
		if err == nil {
			log.Println("DynamoDB is ready!")
			break
		}
		time.Sleep(2 * time.Second)
	}

	createTables(ctx, client)
	seedData(ctx, client)
	log.Println("DynamoDB initialization complete!")
}

func tableExists(ctx context.Context, client *dynamodb.Client, name string) bool {
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(name)})
	return err == nil
}

func createTables(ctx context.Context, client *dynamodb.Client) {
	// 1. Products
	if !tableExists(ctx, client, "Products") {
		log.Println("Creating table Products...")
		_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String("Products"),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("sku"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("is_featured"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("created_at"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("brand"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
			},
			GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
				{
					IndexName: aws.String("sku-index"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("sku"), KeyType: types.KeyTypeHash},
					},
					Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				},
				{
					IndexName: aws.String("featured-index"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("is_featured"), KeyType: types.KeyTypeHash},
						{AttributeName: aws.String("created_at"), KeyType: types.KeyTypeRange},
					},
					Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				},
				{
					IndexName: aws.String("brand-index"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("brand"), KeyType: types.KeyTypeHash},
						{AttributeName: aws.String("created_at"), KeyType: types.KeyTypeRange},
					},
					Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		if err != nil {
			log.Printf("Error creating Products table: %v", err)
		}
	}

	// 2. Categories
	if !tableExists(ctx, client, "Categories") {
		log.Println("Creating table Categories...")
		_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String("Categories"),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("name"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
			},
			GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
				{
					IndexName: aws.String("name-index"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("name"), KeyType: types.KeyTypeHash},
					},
					Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		if err != nil {
			log.Printf("Error creating Categories table: %v", err)
		}
	}

	// 3. ProductCategories
	if !tableExists(ctx, client, "ProductCategories") {
		log.Println("Creating table ProductCategories...")
		_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String("ProductCategories"),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("category_id"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("product_id"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("category_id"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("product_id"), KeyType: types.KeyTypeRange},
			},
			GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
				{
					IndexName: aws.String("product-index"),
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("product_id"), KeyType: types.KeyTypeHash},
						{AttributeName: aws.String("category_id"), KeyType: types.KeyTypeRange},
					},
					Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		if err != nil {
			log.Printf("Error creating ProductCategories table: %v", err)
		}
	}

	// 4. Inventory
	if !tableExists(ctx, client, "Inventory") {
		log.Println("Creating table Inventory...")
		_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String("Inventory"),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
			},
			BillingMode: types.BillingModePayPerRequest,
		})
		if err != nil {
			log.Printf("Error creating Inventory table: %v", err)
		}
	}
}

func seedData(ctx context.Context, client *dynamodb.Client) {
	// Check if Products table already has items
	scanOut, err := client.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String("Products"),
		Limit:     aws.Int32(1),
	})
	if err == nil && scanOut.Count > 0 {
		log.Println("Products already seeded, skipping seed.")
		return
	}

	log.Println("Seeding initial products & categories...")
	now := time.Now().UTC().Format(time.RFC3339)

	catElectronicsID := uuid.New().String()
	catApparelID := uuid.New().String()

	categories := []Category{
		{ID: catElectronicsID, Name: "Electronics", Slug: "electronics", CreatedAt: now, UpdatedAt: now},
		{ID: catApparelID, Name: "Apparel & Accessories", Slug: "apparel-accessories", CreatedAt: now, UpdatedAt: now},
	}

	for _, c := range categories {
		item, _ := attributevalue.MarshalMap(c)
		_, _ = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("Categories"),
			Item:      item,
		})
	}

	products := []struct {
		Product   Product
		CatID     string
	}{
		{
			Product: Product{
				ID:          uuid.New().String(),
				Name:        "Wireless Noise-Canceling Headphones",
				Price:       199.99,
				Quantity:    50,
				Description: "Premium wireless over-ear headphones with active noise cancellation and 30-hour battery life.",
				Images:      []string{"https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=800&auto=format&fit=crop&q=60"},
				Brand:       "AudioPro",
				SKU:         "AP-WH-100",
				CategoryIDs: []string{catElectronicsID},
				IsFeatured:  "true",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			CatID: catElectronicsID,
		},
		{
			Product: Product{
				ID:          uuid.New().String(),
				Name:        "Minimalist Mechanical Keyboard",
				Price:       129.50,
				Quantity:    35,
				Description: "Sleek 75% layout mechanical keyboard with hot-swappable switches and RGB backlighting.",
				Images:      []string{"https://images.unsplash.com/photo-1587829741301-dc798b83add3?w=800&auto=format&fit=crop&q=60"},
				Brand:       "KeyCraft",
				SKU:         "KC-MK-75",
				CategoryIDs: []string{catElectronicsID},
				IsFeatured:  "true",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			CatID: catElectronicsID,
		},
		{
			Product: Product{
				ID:          uuid.New().String(),
				Name:        "Classic Urban Sneaker",
				Price:       89.00,
				Quantity:    80,
				Description: "Everyday comfort sneakers crafted with premium breathable canvas and cushioned soles.",
				Images:      []string{"https://images.unsplash.com/photo-1549298916-b41d501d3772?w=800&auto=format&fit=crop&q=60"},
				Brand:       "Stride",
				SKU:         "ST-SNK-01",
				CategoryIDs: []string{catApparelID},
				IsFeatured:  "true",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			CatID: catApparelID,
		},
	}

	for _, p := range products {
		item, _ := attributevalue.MarshalMap(p.Product)
		_, _ = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("Products"),
			Item:      item,
		})

		// Category link
		link := ProductCategory{
			CategoryID: p.CatID,
			ProductID:  p.Product.ID,
			CreatedAt:  now,
		}
		lItem, _ := attributevalue.MarshalMap(link)
		_, _ = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("ProductCategories"),
			Item:      lItem,
		})

		// Inventory
		inv := Inventory{
			ID:        p.Product.ID,
			Available: p.Product.Quantity,
			Reserved:  0,
			UpdatedAt: now,
		}
		invItem, _ := attributevalue.MarshalMap(inv)
		_, _ = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String("Inventory"),
			Item:      invItem,
		})
	}
	log.Printf("Successfully seeded %d sample products!", len(products))
}
