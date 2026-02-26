package sender

import (
	"context"
	"time"
)

type SendResult struct {
	MessageID string
	SentAt    time.Time
}

type EmailSender interface {
	SendEmail(ctx context.Context, to, subject, body string) (SendResult, error)
}

type SMSSender interface {
	SendSMS(ctx context.Context, to, msg string) (SendResult, error)
}
