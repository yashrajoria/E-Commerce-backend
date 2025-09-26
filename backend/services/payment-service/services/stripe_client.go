package services

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/stripe/stripe-go/v80"
	"github.com/stripe/stripe-go/v80/paymentintent"
	"github.com/stripe/stripe-go/v80/webhook"
)

type StripeService struct {
    SecretKey  string
    WebhookKey string
}

func NewStripeService(secretKey, webhookKey string) *StripeService {
    stripe.Key = secretKey
    return &StripeService{SecretKey: secretKey, WebhookKey: webhookKey}
}

func (s *StripeService) CreatePaymentIntent(amount int64, currency string) (string, error) {
    params := &stripe.PaymentIntentParams{
        Amount:   stripe.Int64(amount),
        Currency: stripe.String(currency),
    }
    pi, err := paymentintent.New(params)
    if err != nil {
        return "", err
    }
    return pi.ID, nil
}

func (s *StripeService) ParseWebhook(r *http.Request) (stripe.Event, error) {
    var event stripe.Event
    payload, err := ioutil.ReadAll(r.Body)
    if err != nil {
        return event, err
    }
    r.Body = ioutil.NopCloser(bytes.NewBuffer(payload))
    sigHeader := r.Header.Get("Stripe-Signature")
    return webhook.ConstructEvent(payload, sigHeader, s.WebhookKey)
}
