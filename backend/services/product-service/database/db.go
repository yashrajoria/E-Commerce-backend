package database

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoClient *mongo.Client
	DB          *mongo.Database
	ctx         context.Context
	cancel      context.CancelFunc
)

// ConnectWithConfig connects to MongoDB using the provided URI and database name.
func ConnectWithConfig(mongoURL, dbName string) error {
	ctx, cancel = context.WithCancel(context.Background())
	clientOptions := options.Client().ApplyURI(mongoURL)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	MongoClient = client
	DB = client.Database(dbName)
	log.Println("✅ Successfully connected to MongoDB!")
	return nil
}

// Close disconnects from MongoDB
func Close() error {
	// Disconnect from MongoDB
	if err := MongoClient.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	log.Println("❌ Disconnected from MongoDB")
	return nil
}
