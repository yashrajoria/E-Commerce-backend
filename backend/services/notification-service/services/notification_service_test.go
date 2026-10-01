package services

import (
	"context"
	"errors"
	"os"
	"testing"

	"notification-service/models"
	"notification-service/sender"

	"go.uber.org/zap"
)

type notificationRepoFake struct {
	claimed    bool
	claimCalls int
	released   int
	delivered  int
}

func (r *notificationRepoFake) SaveLog(context.Context, *models.NotificationLog) error { return nil }
func (r *notificationRepoFake) ClaimEvent(context.Context, string) (bool, error) {
	r.claimCalls++
	return r.claimed, nil
}
func (r *notificationRepoFake) MarkEventDelivered(context.Context, string) error {
	r.delivered++
	return nil
}
func (r *notificationRepoFake) ReleaseEvent(context.Context, string) error {
	r.released++
	return nil
}
func (r *notificationRepoFake) GetLogs(context.Context, models.NotificationFilter) ([]models.NotificationLog, int64, error) {
	return nil, 0, nil
}
func (r *notificationRepoFake) GetLogByID(context.Context, int64) (*models.NotificationLog, error) {
	return nil, nil
}

type notificationSenderFake struct {
	calls int
	err   error
}

func (s *notificationSenderFake) SendEmail(context.Context, string, string, string) (sender.SendResult, error) {
	s.calls++
	return sender.SendResult{}, s.err
}

func TestDuplicateNotificationSkipsDelivery(t *testing.T) {
	changeToServiceRoot(t)
	repo := &notificationRepoFake{claimed: false}
	email := &notificationSenderFake{}
	service, err := NewNotificationService(repo, email, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	err = service.ProcessEvent(context.Background(), &models.EventPayload{
		EventID: "notification-1", EventType: models.TypeUserRegistered,
		Data: map[string]interface{}{"email": "user@example.com"},
	})
	if err != nil || email.calls != 0 {
		t.Fatalf("duplicate notification sent or failed: err=%v calls=%d", err, email.calls)
	}
}

func TestNotificationDeliveryFailureReleasesClaim(t *testing.T) {
	changeToServiceRoot(t)
	repo := &notificationRepoFake{claimed: true}
	email := &notificationSenderFake{err: errors.New("temporary SMTP failure")}
	service, err := NewNotificationService(repo, email, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	err = service.ProcessEvent(context.Background(), &models.EventPayload{
		EventID: "notification-2", EventType: models.TypeUserRegistered,
		Data: map[string]interface{}{"email": "user@example.com"},
	})
	if err == nil {
		t.Fatal("transient delivery failure was acknowledged")
	}
	if repo.released != 1 || repo.delivered != 0 {
		t.Fatalf("claim state was not retryable: released=%d delivered=%d", repo.released, repo.delivered)
	}
}

func TestNotificationDeliveryMarksClaimDelivered(t *testing.T) {
	changeToServiceRoot(t)
	repo := &notificationRepoFake{claimed: true}
	email := &notificationSenderFake{}
	service, err := NewNotificationService(repo, email, nil, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	err = service.ProcessEvent(context.Background(), &models.EventPayload{
		EventID: "notification-success", EventType: models.TypeUserRegistered,
		Data: map[string]interface{}{"email": "user@example.com"},
	})
	if err != nil {
		t.Fatalf("successful notification returned error: %v", err)
	}
	if repo.delivered != 1 || repo.released != 0 || email.calls != 1 {
		t.Fatalf("unexpected delivery state: delivered=%d released=%d calls=%d", repo.delivered, repo.released, email.calls)
	}
}

func changeToServiceRoot(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("templates"); err == nil {
		return
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatal(err)
	}
}
