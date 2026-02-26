package models

import "time"

const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"

	StatusSent   = "sent"
	StatusFailed = "failed"

	TypeOrderCreated   = "order_created"
	TypeOrderShipped   = "order_shipped"
	TypeOrderDelivered = "order_delivered"
	TypeUserRegistered = "user_registered"
	TypeCouponApplied  = "coupon_applied"
	TypePaymentFailed  = "payment_failed"
	TypeOTPSMS         = "otp_sms"
)

type NotificationLog struct {
	ID         int64     `json:"id" db:"id" gorm:"primaryKey;autoIncrement"`
	UserID     int64     `json:"user_id" db:"user_id"`
	Recipient  string    `json:"recipient" db:"recipient"`
	Type       string    `json:"type" db:"type"`
	Channel    string    `json:"channel" db:"channel"`
	Status     string    `json:"status" db:"status"`
	Error      string    `json:"error,omitempty" db:"error"`
	RetryCount int       `json:"retry_count" db:"retry_count"`
	CreatedAt  time.Time `json:"created_at" db:"created_at" gorm:"autoCreateTime"`
}

type NotificationFilter struct {
	UserID   int64
	Status   string
	Channel  string
	Page     int
	PageSize int
}

type EventPayload struct {
	EventType string                 `json:"event_type"`
	UserID    int64                  `json:"user_id"`
	Recipient string                 `json:"recipient"`
	Data      map[string]interface{} `json:"data"`
}
