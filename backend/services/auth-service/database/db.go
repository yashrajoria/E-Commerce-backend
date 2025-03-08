package database

import (
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB // ✅ Define a global DB variable

func Connect() error {
	dsn := "host=localhost user=postgres password=admin dbname=ecommerce port=5432 sslmode=disable TimeZone=Asia/Kolkata"
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Println("Failed to connect to database:", err)
		return err
	}
	log.Println("Connected to PostgreSQL successfully!")
	return nil
}
