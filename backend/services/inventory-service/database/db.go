package database

import (
	"log"

	"github.com/yashrajoria/common/db"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	MongoClient *mongo.Client
	DB          *mongo.Database
)

func Connect() error {
	var err error
	MongoClient, DB, err = db.ConnectMongo()
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
