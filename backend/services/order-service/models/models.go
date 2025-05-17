package models

import (
	"time"

	"github.com/google/uuid"
)

type Order struct {
	ID        uuid.UUID   `json:"id" gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	UserID    uuid.UUID   `json:"user_id" gorm:"type:uuid;not null"`
	Amount    int         `json:"amount"` // total amount in paisa
	Status    string      `json:"status"` // pending, paid, failed, etc.
	CreatedAt time.Time   `json:"created_at" gorm:"autoCreateTime"`
	Items     []OrderItem `json:"items" gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE"`
}

type OrderItem struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	OrderID   uuid.UUID `json:"order_id" gorm:"type:uuid;not null"`
	ProductID uuid.UUID `json:"product_id" gorm:"type:uuid;not null"`
	Quantity  int       `json:"quantity"`
	Price     int       `json:"price"` // price per item at time of order (in paisa)
}
