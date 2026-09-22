package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"order-service/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type publisherRepo struct {
	mu         sync.Mutex
	event      models.OutboxEvent
	markErr    error
	claimCount int
}

func (r *publisherRepo) Create(context.Context, *gorm.DB, *models.OutboxEvent) error { return nil }
func (r *publisherRepo) Claim(ctx context.Context, limit int) ([]models.OutboxEvent, error) {
	return r.ClaimWithLease(ctx, limit, "compat", time.Minute)
}
func (r *publisherRepo) ClaimWithLease(ctx context.Context, limit int, owner string, lease time.Duration) ([]models.OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 || (r.event.Status != models.OutboxStatusPending && !(r.event.Status == models.OutboxStatusProcessing && r.event.LeaseExpiresAt != nil && !r.event.LeaseExpiresAt.After(time.Now()))) {
		return nil, nil
	}
	r.event.Status = models.OutboxStatusProcessing
	r.event.Attempts++
	r.event.LeaseOwner = &owner
	expires := time.Now().Add(lease)
	r.event.LeaseExpiresAt = &expires
	r.claimCount++
	return []models.OutboxEvent{r.event}, nil
}
func (r *publisherRepo) MarkPublished(context.Context, uuid.UUID) error { return nil }
func (r *publisherRepo) MarkPublishedByOwner(_ context.Context, id uuid.UUID, owner string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.markErr != nil {
		return r.markErr
	}
	if r.event.ID != id || r.event.Status != models.OutboxStatusProcessing || r.event.LeaseOwner == nil || *r.event.LeaseOwner != owner {
		return errors.New("lost lease")
	}
	r.event.Status = models.OutboxStatusPublished
	return nil
}
func (r *publisherRepo) MarkFailed(context.Context, uuid.UUID, string, time.Time) error { return nil }
func (r *publisherRepo) MarkFailedByOwner(_ context.Context, id uuid.UUID, owner, eventErr string, availableAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.event.ID != id || r.event.Status != models.OutboxStatusProcessing || r.event.LeaseOwner == nil || *r.event.LeaseOwner != owner {
		return errors.New("lost lease")
	}
	r.event.Status = models.OutboxStatusPending
	r.event.LastError = &eventErr
	r.event.AvailableAt = availableAt
	r.event.LeaseOwner = nil
	r.event.LeaseExpiresAt = nil
	return nil
}

type fakeOutboxTransport struct {
	mu    sync.Mutex
	count int
	err   error
}

type fakeSNSOutboxPublisher struct {
	count   int
	topic   string
	payload []byte
}

func (f *fakeSNSOutboxPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	f.count++
	f.topic = topic
	f.payload = append([]byte(nil), payload...)
	return nil
}

func (f *fakeOutboxTransport) Publish(context.Context, string, []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count++
	return f.err
}

func newPublisherRepo(destination string) *publisherRepo {
	return &publisherRepo{event: models.OutboxEvent{
		ID: uuid.New(), Status: models.OutboxStatusPending, AvailableAt: time.Now().Add(-time.Second),
		DestinationType: models.OutboxDestinationSQS, Destination: destination, Payload: []byte(`{"event":"test"}`),
	}}
}

func testPublisher(repo *publisherRepo, transport *fakeOutboxTransport) *OutboxPublisher {
	return NewOutboxPublisherWithConfig(repo, transport, nil, "worker-1", OutboxPublisherConfig{
		BatchSize: 1, LeaseDuration: time.Minute, RetryBase: time.Hour, RetryMax: time.Hour,
	})
}

func TestOutboxPublisherFailureSchedulesRetry(t *testing.T) {
	repo := newPublisherRepo("queue")
	transport := &fakeOutboxTransport{err: errors.New("AWS unavailable")}
	publisher := testPublisher(repo, transport)

	if _, err := publisher.PublishOnce(context.Background()); err == nil {
		t.Fatal("expected publish error")
	}
	if repo.event.Status != models.OutboxStatusPending {
		t.Fatalf("expected pending retry, got %s", repo.event.Status)
	}
	if repo.event.LastError == nil || *repo.event.LastError != "AWS unavailable" {
		t.Fatalf("expected persisted publish error, got %v", repo.event.LastError)
	}
	if !repo.event.AvailableAt.After(time.Now()) {
		t.Fatalf("expected future retry time, got %v", repo.event.AvailableAt)
	}
}

func TestOutboxPublisherRepublishesAfterCrashBeforeStatusUpdate(t *testing.T) {
	repo := newPublisherRepo("queue")
	repo.markErr = errors.New("simulated crash before status update")
	transport := &fakeOutboxTransport{}
	publisher := testPublisher(repo, transport)

	if _, err := publisher.PublishOnce(context.Background()); err == nil {
		t.Fatal("expected status update error")
	}
	repo.markErr = nil
	expired := time.Now().Add(-time.Second)
	repo.event.LeaseExpiresAt = &expired

	if _, err := publisher.PublishOnce(context.Background()); err != nil {
		t.Fatalf("republish failed: %v", err)
	}
	if transport.count != 2 {
		t.Fatalf("expected duplicate publish after crash, got %d publishes", transport.count)
	}
	if repo.event.Status != models.OutboxStatusPublished {
		t.Fatalf("expected published status, got %s", repo.event.Status)
	}
}

func TestOutboxPublisherReclaimsExpiredLease(t *testing.T) {
	repo := newPublisherRepo("queue")
	oldOwner := "old-worker"
	expired := time.Now().Add(-time.Second)
	repo.event.Status = models.OutboxStatusProcessing
	repo.event.LeaseOwner = &oldOwner
	repo.event.LeaseExpiresAt = &expired
	transport := &fakeOutboxTransport{}

	if _, err := testPublisher(repo, transport).PublishOnce(context.Background()); err != nil {
		t.Fatalf("expired lease was not reclaimed: %v", err)
	}
	if repo.claimCount != 1 || repo.event.Status != models.OutboxStatusPublished {
		t.Fatalf("expected reclaimed and published event, claims=%d status=%s", repo.claimCount, repo.event.Status)
	}
}

func TestOutboxPublisherConcurrentClaimsOnlyPublishOnce(t *testing.T) {
	repo := newPublisherRepo("queue")
	transport := &fakeOutboxTransport{}
	first := testPublisher(repo, transport)
	second := NewOutboxPublisherWithConfig(repo, transport, nil, "worker-2", first.config)
	var wg sync.WaitGroup
	for _, publisher := range []*OutboxPublisher{first, second} {
		wg.Add(1)
		go func(publisher *OutboxPublisher) {
			defer wg.Done()
			_, _ = publisher.PublishOnce(context.Background())
		}(publisher)
	}
	wg.Wait()
	if repo.claimCount != 1 || transport.count != 1 {
		t.Fatalf("expected one claim and publish, claims=%d publishes=%d", repo.claimCount, transport.count)
	}
}

func TestOutboxPublisherPublishesSNSEvents(t *testing.T) {
	repo := newPublisherRepo("arn:aws:sns:local:123:events")
	repo.event.DestinationType = models.OutboxDestinationSNS
	sns := &fakeSNSOutboxPublisher{}
	publisher := NewOutboxPublisherWithConfig(repo, nil, sns, "worker-1", OutboxPublisherConfig{BatchSize: 1})

	if count, err := publisher.PublishOnce(context.Background()); err != nil || count != 1 {
		t.Fatalf("SNS publish failed: count=%d err=%v", count, err)
	}
	if sns.count != 1 || sns.topic != repo.event.Destination || string(sns.payload) != string(repo.event.Payload) {
		t.Fatalf("unexpected SNS publish: count=%d topic=%q payload=%q", sns.count, sns.topic, sns.payload)
	}
}

func TestOutboxPublisherReturnsMarkFailure(t *testing.T) {
	repo := newPublisherRepo("queue")
	repo.markErr = errors.New("lease lost")
	publisher := testPublisher(repo, &fakeOutboxTransport{})

	if _, err := publisher.PublishOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "mark event") {
		t.Fatalf("expected mark-published error, got %v", err)
	}
}
