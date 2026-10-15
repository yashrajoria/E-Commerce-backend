// Package sender defines delivery provider interfaces and implementations for email and SMS.
package sender

import (
	"context"
	"time"
)

// SendResult represents send result.
type SendResult struct {
	MessageID string
	SentAt    time.Time
}

// EmailSender defines the interface contract for email sender.
type EmailSender interface {
	SendEmail(ctx context.Context, to, subject, body string) (SendResult, error)
}

// SMSSender defines the interface contract for sms sender.
type SMSSender interface {
	SendSMS(ctx context.Context, to, msg string) (SendResult, error)
}
