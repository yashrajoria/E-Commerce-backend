package database

import (
	"auth-service/models"
	"log"

	"github.com/yashrajoria/common/db"

	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() error {
	var err error
	DB, err = db.ConnectPostgres(&models.User{}, &models.Address{})
	if err != nil {
		log.Println("❌ Failed to connect to PostgreSQL:", err)
		return err
	}
	return nil
}
