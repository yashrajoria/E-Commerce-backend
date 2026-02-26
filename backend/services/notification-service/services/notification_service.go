package services

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"notification-service/models"
	"notification-service/repository"
	"notification-service/sender"
	"time"

	"go.uber.org/zap"
)

type NotificationService interface {
	ProcessEvent(ctx context.Context, payload *models.EventPayload) error
	GetLogs(ctx context.Context, filter models.NotificationFilter) ([]models.NotificationLog, int64, error)
}

type eventConfig struct {
	tmplFile string
	channels []string
	subject  string
	toKeys   map[string]string // channel → payload data key for recipient
}

var eventConfigs = map[string]eventConfig{
	models.TypeOrderCreated: {
		tmplFile: "templates/order_created.html",
		channels: []string{models.ChannelEmail, models.ChannelSMS},
		subject:  "Order Confirmed!",
		toKeys:   map[string]string{models.ChannelEmail: "email", models.ChannelSMS: "phone"},
	},
	models.TypeOrderShipped: {
		tmplFile: "templates/order_shipped.html",
		channels: []string{models.ChannelEmail},
		subject:  "Your order has shipped!",
		toKeys:   map[string]string{models.ChannelEmail: "email"},
	},
	models.TypeOrderDelivered: {
		tmplFile: "templates/order_delivered.html",
		channels: []string{models.ChannelEmail},
		subject:  "Your order has been delivered!",
		toKeys:   map[string]string{models.ChannelEmail: "email"},
	},
	models.TypeUserRegistered: {
		tmplFile: "templates/welcome.html",
		channels: []string{models.ChannelEmail},
		subject:  "Welcome!",
		toKeys:   map[string]string{models.ChannelEmail: "email"},
	},
	models.TypeCouponApplied: {
		tmplFile: "templates/coupon_applied.html",
		channels: []string{models.ChannelEmail},
		subject:  "Coupon Applied!",
		toKeys:   map[string]string{models.ChannelEmail: "email"},
	},
	models.TypePaymentFailed: {
		tmplFile: "templates/payment_failed.html",
		channels: []string{models.ChannelEmail, models.ChannelSMS},
		subject:  "Payment Failed",
		toKeys:   map[string]string{models.ChannelEmail: "email", models.ChannelSMS: "phone"},
	},
	models.TypeOTPSMS: {
		tmplFile: "templates/otp_sms.txt",
		channels: []string{models.ChannelSMS},
		toKeys:   map[string]string{models.ChannelSMS: "phone"},
	},
}

type notificationService struct {
	repo        repository.NotificationRepository
	emailSender sender.EmailSender
	smsSender   sender.SMSSender
	templates   map[string]*template.Template
	logger      *zap.Logger
}

func NewNotificationService(
	repo repository.NotificationRepository,
	emailSender sender.EmailSender,
	// smsSender sender.SMSSender,
	logger *zap.Logger,
) (NotificationService, error) {
	tmpls := make(map[string]*template.Template)
	for eventType, cfg := range eventConfigs {
		tmpl, err := template.ParseFiles(cfg.tmplFile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template for %s: %w", eventType, err)
		}
		tmpls[eventType] = tmpl
	}
	return &notificationService{
		repo:        repo,
		emailSender: emailSender,
		// smsSender:   smsSender,
		templates: tmpls,
		logger:    logger,
	}, nil
}

func (s *notificationService) ProcessEvent(ctx context.Context, payload *models.EventPayload) error {
	cfg, ok := eventConfigs[payload.EventType]
	if !ok {
		return fmt.Errorf("unsupported event type: %s", payload.EventType)
	}

	// Render template once — reused across channels
	tmpl := s.templates[payload.EventType]
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, payload.Data); err != nil {
		return fmt.Errorf("template render failed: %w", err)
	}
	renderedBody := buf.String()

	// Send on each configured channel
	for _, channel := range cfg.channels {
		toKey := cfg.toKeys[channel]
		to, ok := payload.Data[toKey].(string)
		if !ok || to == "" {
			// Fall back to top-level Recipient field
			to = payload.Recipient
		}
		if to == "" {
			s.logger.Warn("missing recipient, skipping channel",
				zap.String("channel", channel),
				zap.String("event", payload.EventType),
			)
			continue
		}

		s.sendWithRetry(ctx, channel, to, cfg.subject, renderedBody, payload)
	}

	return nil
}

func (s *notificationService) sendWithRetry(
	ctx context.Context,
	channel, to, subject, body string,
	payload *models.EventPayload,
) {
	var lastErr error
	var messageID string

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		var result sender.SendResult
		switch channel {
		case models.ChannelEmail:
			result, lastErr = s.emailSender.SendEmail(ctx, to, subject, body)
		case models.ChannelSMS:
			result, lastErr = s.smsSender.SendSMS(ctx, to, body)
		}

		if lastErr == nil {
			messageID = result.MessageID
			break
		}

		s.logger.Warn("send attempt failed",
			zap.String("channel", channel),
			zap.String("event", payload.EventType),
			zap.Int("attempt", attempt+1),
			zap.Error(lastErr),
		)
	}

	status := models.StatusSent
	errMsg := ""
	if lastErr != nil {
		status = models.StatusFailed
		errMsg = lastErr.Error()
	}

	logEntry := &models.NotificationLog{
		UserID:    payload.UserID,
		Recipient: to,
		Type:      payload.EventType,
		Channel:   channel,
		Status:    status,
		Error:     errMsg,
	}

	s.logger.Info("notification sent",
		zap.String("event", payload.EventType),
		zap.String("channel", channel),
		zap.String("status", status),
		zap.String("message_id", messageID),
	)

	if err := s.repo.SaveLog(ctx, logEntry); err != nil {
		s.logger.Error("failed to save notification log", zap.Error(err))
	}
}

func (s *notificationService) GetLogs(ctx context.Context, filter models.NotificationFilter) ([]models.NotificationLog, int64, error) {
	return s.repo.GetLogs(ctx, filter)
}
