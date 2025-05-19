package database

import (
	"log"

	"github.com/yashrajoria/common/db"
	"github.com/yashrajoria/order-service/models"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() error {
	var err error
	DB, err = db.ConnectPostgres(&models.Order{}, &models.OrderItem{})
	if err != nil {
		log.Println("❌ Failed to connect to PostgreSQL:", err)
		return err
	}
	return nil
}
