package db

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client
var DB *mongo.Database

func Connect() error {
	uri := "sadasdasjpodaspodajspodaspdj/"

	clientOptions := options.Client().ApplyURI(uri)

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database to ensure connectivity
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	mongoClient = client
	DB = client.Database("test")
	log.Println(DB)

	return nil
}

// Close disconnects from MongoDB
func Close() error {
	// Disconnect from MongoDB
	err := mongoClient.Disconnect(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	log.Println("Disconnected from MongoDB!")
	return nil
}
