package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoClient *mongo.Client
	DB          *mongo.Database
)

// ConnectMongo connects to MongoDB using environment variables and returns a *mongo.Client and *mongo.Database.
func ConnectMongo() (*mongo.Client, *mongo.Database, error) {
	_ = godotenv.Load()

	uri := os.Getenv("MONGO_DB_URL")
	dbName := os.Getenv("MONGO_DB_NAME")
	if uri == "" || dbName == "" {
		return nil, nil, fmt.Errorf("MONGO_DB_URL or MONGO_DB_NAME not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(uri)
	// Optionally set pool sizes from env
	if maxPool := os.Getenv("MONGO_MAX_POOL_SIZE"); maxPool != "" {
		if max, err := parseUintEnv(maxPool); err == nil {
			clientOptions.SetMaxPoolSize(uint64(max))
		}
	}
	if minPool := os.Getenv("MONGO_MIN_POOL_SIZE"); minPool != "" {
		if min, err := parseUintEnv(minPool); err == nil {
			clientOptions.SetMinPoolSize(uint64(min))
		}
	}

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	db := client.Database(dbName)
	log.Println("✅ Connected to MongoDB successfully!")
	return client, db, nil
}

func parseUintEnv(val string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(val, "%d", &n)
	return n, err
}
func Connect() error {
	var err error
	MongoClient, DB, err = ConnectMongo()
	if err != nil {
		log.Println("❌ Failed to connect to MongoDB:", err)
		return err
	}
	return nil
}

// Close disconnects from MongoDB
func Close() error {
	// Disconnect from MongoDB
	err := MongoClient.Disconnect(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
	}

	log.Println("Disconnected from MongoDB!")
	return nil
}
