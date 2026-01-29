package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoClient *mongo.Client
	DB          *mongo.Database
)

// ConnectWithConfig connects to MongoDB using the provided URI and database name.
func ConnectWithConfig(mongoURL, dbName string) error {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel() // Always call cancel

	clientOptions := options.Client().ApplyURI(mongoURL)

	client, err := mongo.Connect(timeoutCtx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	if err := client.Ping(timeoutCtx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	MongoClient = client
	DB = client.Database(dbName)
	log.Println("✅ Successfully connected to MongoDB!")
	return nil
}

// Close disconnects from MongoDB
func Close() error {
	// Create a timeout context for graceful disconnect
	disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer disconnectCancel()

	// Disconnect from MongoDB
	if err := MongoClient.Disconnect(disconnectCtx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	log.Println("✅ Disconnected from MongoDB")
	return nil
}
