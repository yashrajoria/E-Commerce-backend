package sender

import (
	"context"
	"fmt"
	"net/smtp"
	"os"
	"time"
)

type SMTPSender struct {
	host     string
	port     string
	username string
	password string
}

func NewSMTPSender() (*SMTPSender, error) {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USER")
	password := os.Getenv("SMTP_PASS")

	if host == "" {
		return nil, fmt.Errorf("SMTP_HOST not set")
	}
	if port == "" {
		return nil, fmt.Errorf("SMTP_PORT not set")
	}
	if username == "" {
		return nil, fmt.Errorf("SMTP_USER not set")
	}
	if password == "" {
		return nil, fmt.Errorf("SMTP_PASS not set")
	}

	return &SMTPSender{host, port, username, password}, nil
}

func (s *SMTPSender) SendEmail(ctx context.Context, to, subject, body string) (SendResult, error) {
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)

	msg := []byte(
		"From: " + s.username + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n" +
			"\r\n" +
			body,
	)

	if err := smtp.SendMail(addr, auth, s.username, []string{to}, msg); err != nil {
		return SendResult{}, fmt.Errorf("smtp send failed: %w", err)
	}

	return SendResult{
		MessageID: fmt.Sprintf("smtp-%d", time.Now().UnixNano()),
		SentAt:    time.Now(),
	}, nil
}
