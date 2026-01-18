package services

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/stripe/stripe-go/v80/checkout/session"

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

func (s *StripeService) CreateCheckoutSession(amount int64, currency, orderID, userID string) (*stripe.CheckoutSession, error) {
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:         stripe.String("http://localhost:3000/payment/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:          stripe.String("http://localhost:3000/payment/cancel"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Order " + orderID),
					},
					UnitAmount: stripe.Int64(amount),
				},
				Quantity: stripe.Int64(1),
			},
		},
	}
	params.AddMetadata("order_id", orderID)
	if userID != "" {
		params.AddMetadata("user_id", userID)
	}

	sess, err := session.New(params)
	if err != nil {
		return nil, err
	}
	return sess, nil
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
