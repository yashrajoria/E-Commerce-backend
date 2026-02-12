package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"product-service/models"

	"github.com/google/uuid"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	ddbrepo "product-service/repository"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func main() {
	var mongoURI, dbName, table string
	flag.StringVar(&mongoURI, "mongo", os.Getenv("MONGO_DB_URL"), "MongoDB URI")
	flag.StringVar(&dbName, "db", os.Getenv("MONGO_DB_NAME"), "MongoDB database name")
	flag.StringVar(&table, "table", os.Getenv("DDB_TABLE_PRODUCTS"), "DynamoDB table name")
	flag.Parse()

	if mongoURI == "" || dbName == "" {
		log.Fatal("MONGO_DB_URL and MONGO_DB_NAME must be set or provided via flags")
	}
	if table == "" {
		table = "Products"
	}

	ctx := context.Background()
	// Connect to Mongo
	clientOpts := options.Client().ApplyURI(mongoURI)
	mclient, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer mclient.Disconnect(ctx)

	coll := mclient.Database(dbName).Collection("products")

	// Connect to AWS / DynamoDB
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	ddbClient := dynamodb.NewFromConfig(awsCfg)
	repo := ddbrepo.NewDynamoAdapter(ddbClient, table)

	// Scan Mongo in batches
	batchSize := int64(500)
	cur, err := coll.Find(ctx, bson.M{}, &options.FindOptions{BatchSize: &batchSize})
	if err != nil {
		log.Fatalf("mongo find: %v", err)
	}
	defer cur.Close(ctx)

	var count int
	for cur.Next(ctx) {
		var p models.Product
		if err := cur.Decode(&p); err != nil {
			log.Printf("decode error: %v", err)
			continue
		}
		// Ensure timestamps
		if p.CreatedAt.IsZero() {
			p.CreatedAt = time.Now().UTC()
		}
		if p.UpdatedAt.IsZero() {
			p.UpdatedAt = p.CreatedAt
		}
		if p.ID == (uuid.UUID{}) {
			p.ID = uuid.New()
		}
		if err := repo.Create(ctx, &p); err != nil {
			log.Printf("failed to write product %s to ddb: %v", p.ID.String(), err)
			continue
		}
		count++
		if count%100 == 0 {
			log.Printf("migrated %d products", count)
		}
	}
	if err := cur.Err(); err != nil {
		log.Fatalf("cursor error: %v", err)
	}
	fmt.Printf("Migration complete. migrated=%d\n", count)
}
