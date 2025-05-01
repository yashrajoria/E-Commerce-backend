package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	mongoClient *mongo.Client
	DB          *mongo.Database
	ctx         context.Context
	cancel      context.CancelFunc
)

// Connect initializes MongoDB connection
func Connect() error {

	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}
	// Load URI from environment variable
	// uri := os.Getenv("MONGO_DB_URL")
	uri := "mongodb+srv://yashrajoria:MvpTpbFRNGqDRhGC@ecommerce.dafay.mongodb.net/"
	if uri == "" {
		return fmt.Errorf("MONGO_DB_URL is not set")
	}

	log.Println((uri))
	// Set timeout for connection
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)

	// MongoDB client options with connection pool settings
	clientOptions := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(100). // Set max connection pool size
		SetMinPoolSize(10)   // Keep some connections warm

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping the database to ensure connectivity
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	mongoClient = client
	DB = client.Database("ecommerce") // Changed from "test" to "ecommerce"
	log.Println("✅ Successfully connected to MongoDB!")

	return nil
}

// Close disconnects from MongoDB
func Close() error {
	defer cancel() // Cancel the global context

	// Disconnect from MongoDB
	if err := mongoClient.Disconnect(ctx); err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	log.Println("❌ Disconnected from MongoDB")
	return nil
}
