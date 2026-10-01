package sender

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"sync"
	"time"
)

type SMTPSender struct {
	host     string
	port     string
	username string
	password string

	mu     sync.Mutex
	client *smtp.Client
}

func NewSMTPSender() (*SMTPSender, error) {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_EMAIL")
	password := os.Getenv("SMTP_PASSWORD")
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

	return &SMTPSender{host: host, port: port, username: username, password: password}, nil
}

func (s *SMTPSender) SendEmail(ctx context.Context, to, subject, body string) (SendResult, error) {
	msg := []byte(
		"From: " + s.username + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n" +
			"\r\n" +
			body,
	)

	// Fast path: reused authenticated connection (Mail/Rcpt/Data + Reset).
	if err := s.sendPooled(ctx, s.username, to, msg); err == nil {
		return SendResult{
			MessageID: fmt.Sprintf("smtp-%d", time.Now().UnixNano()),
			SentAt:    time.Now().UTC(),
		}, nil
	} else {
		// Fall through to one-shot SendMail so a stale pooled connection
		// never drops a notification — SQS retries on returned error.
		s.closePooled()
		addr := fmt.Sprintf("%s:%s", s.host, s.port)
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if sendErr := smtp.SendMail(addr, auth, s.username, []string{to}, msg); sendErr != nil {
			// Preserve pooled error context when both paths fail.
			return SendResult{}, fmt.Errorf("smtp send failed (pooled: %v; oneshot: %w)", err, sendErr)
		}
	}

	return SendResult{
		MessageID: fmt.Sprintf("smtp-%d", time.Now().UnixNano()),
		SentAt:    time.Now().UTC(),
	}, nil
}

// sendPooled reuses a single authenticated SMTP connection across emails.
// Serialized by mutex — notification volume is low and correctness (no
// interleaved Mail/Rcpt/Data) matters more than parallel sends.
func (s *SMTPSender) sendPooled(ctx context.Context, from, to string, msg []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	c, err := s.pooledClient()
	if err != nil {
		return err
	}

	if err := c.Mail(from); err != nil {
		s.closeLocked()
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		s.closeLocked()
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		s.closeLocked()
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		s.closeLocked()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		s.closeLocked()
		return fmt.Errorf("smtp data close: %w", err)
	}
	// Reset keeps the connection (TCP+TLS+auth) ready for the next mail.
	if err := c.Reset(); err != nil {
		s.closeLocked()
		return fmt.Errorf("smtp reset: %w", err)
	}
	return nil
}

func (s *SMTPSender) pooledClient() (*smtp.Client, error) {
	if s.client != nil {
		if err := s.client.Noop(); err == nil {
			return s.client, nil
		}
		s.closeLocked()
	}
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	c, err := smtp.Dial(addr)
	if err != nil {
		return nil, fmt.Errorf("smtp dial: %w", err)
	}
	if err := c.Hello("shopswift"); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("smtp hello: %w", err)
	}
	// STARTTLS when advertised (port 587); SendMail does the same opportunistically.
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	}
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err := c.Auth(auth); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("smtp auth: %w", err)
	}
	s.client = c
	return c, nil
}

func (s *SMTPSender) closePooled() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

func (s *SMTPSender) closeLocked() {
	if s.client != nil {
		_ = s.client.Close()
		s.client = nil
	}
}
