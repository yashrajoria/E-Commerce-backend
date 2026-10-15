package services

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"order-service/payment/models"
	"os"
	"strings"

	"github.com/stripe/stripe-go/v86/checkout/session"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/paymentintent"
	"github.com/stripe/stripe-go/v86/refund"
	"github.com/stripe/stripe-go/v86/webhook"
)

// frontendURL returns the storefront base URL used for Stripe success/cancel
// redirects. Mirrors controllers.PaymentController.frontendURL() — kept in
// sync here since the async SQS checkout path builds sessions independently
// of that handler.
func frontendURL() string {
	for _, key := range []string{"FRONTEND_URL", "STOREFRONT_URL"} {
		if url := strings.TrimRight(strings.TrimSpace(os.Getenv(key)), "/"); url != "" {
			return url
		}
	}
	return "http://localhost:3001"
}

type StripeService struct {
	SecretKey  string
	WebhookKey string
}

func NewStripeService(secretKey, webhookKey string) *StripeService {
	stripe.Key = secretKey
	// Explicit retries: stripe-go defaults to 2 network retries, but pin it
	// here so transient Stripe timeouts don't become single-try failures.
	// SQS redelivery remains the outer retry (payment claim is idempotent).
	if be := stripe.GetBackend(stripe.APIBackend); be != nil {
		be.SetMaxNetworkRetries(2)
	}
	return &StripeService{SecretKey: secretKey, WebhookKey: webhookKey}
}

func (s *StripeService) CreatePaymentIntent(amount int64, currency string) (*stripe.PaymentIntent, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amount),
		Currency: stripe.String(currency),
	}
	pi, err := paymentintent.New(params)
	if err != nil {
		return nil, err
	}
	return pi, nil
}

func (s *StripeService) CreateCheckoutSession(amount int64, currency, orderID, userID string) (*stripe.CheckoutSession, error) {
	return s.CreateCheckoutSessionWithDetails(amount, currency, orderID, userID, "", "", nil)
}

func (s *StripeService) CreateCheckoutSessionWithIdempotency(amount int64, currency, orderID, userID, idempotencyKey string) (*stripe.CheckoutSession, error) {
	return s.CreateCheckoutSessionWithDetails(amount, currency, orderID, userID, idempotencyKey, "", nil)
}

func (s *StripeService) CreateCheckoutSessionWithDetails(amount int64, currency, orderID, userID, idempotencyKey, email string, shipping *models.ShippingAddress) (*stripe.CheckoutSession, error) {
	frontend := frontendURL()
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		Mode:               stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL:         stripe.String(frontend + "/payment/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:          stripe.String(frontend + "/payment/cancel"),
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

	if email != "" {
		params.CustomerEmail = stripe.String(email)
	}

	if shipping != nil {
		fullName := strings.TrimSpace(shipping.FirstName + " " + shipping.LastName)
		if fullName == "" {
			fullName = "Valued Customer"
		}
		country := strings.ToUpper(strings.TrimSpace(shipping.Country))
		if country == "" || len(country) != 2 {
			country = "GB"
		}

		params.PaymentIntentData = &stripe.CheckoutSessionPaymentIntentDataParams{
			Shipping: &stripe.ShippingDetailsParams{
				Name: stripe.String(fullName),
				Address: &stripe.AddressParams{
					Line1:      stripe.String(shipping.Line1),
					Line2:      stripe.String(shipping.Line2),
					City:       stripe.String(shipping.City),
					State:      stripe.String(shipping.State),
					PostalCode: stripe.String(shipping.PostalCode),
					Country:    stripe.String(country),
				},
			},
		}
		if shipping.Phone != "" {
			params.PaymentIntentData.Shipping.Phone = stripe.String(shipping.Phone)
		}
	}

	params.AddMetadata("order_id", orderID)
	if userID != "" {
		params.AddMetadata("user_id", userID)
	}
	if idempotencyKey != "" {
		params.SetIdempotencyKey(idempotencyKey)
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
	return webhook.ConstructEventWithOptions(payload, sigHeader, s.WebhookKey, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
}

// PaymentIntentID resolves the PaymentIntent behind a stored Stripe id, which is a
// Checkout Session id (cs_...) or already a PaymentIntent id (pi_...).
func (s *StripeService) PaymentIntentID(stripeID string) (string, error) {
	if strings.HasPrefix(stripeID, "pi_") {
		return stripeID, nil
	}
	sess, err := session.Get(stripeID, nil)
	if err != nil {
		return "", err
	}
	if sess.PaymentIntent == nil || sess.PaymentIntent.ID == "" {
		return "", fmt.Errorf("checkout session %s has no payment intent", stripeID)
	}
	return sess.PaymentIntent.ID, nil
}

// Refund creates a Stripe refund for a PaymentIntent with optional partial amount and reason.
// The idempotency key makes retries return the same refund.
func (s *StripeService) Refund(paymentIntentID string, amount int64, reason string, idempotencyKey string) (*stripe.Refund, error) {
	params := &stripe.RefundParams{
		PaymentIntent: stripe.String(paymentIntentID),
	}
	if amount > 0 {
		params.Amount = stripe.Int64(amount)
	}
	if reason != "" {
		switch reason {
		case "duplicate", "fraudulent", "requested_by_customer":
			params.Reason = stripe.String(reason)
		default:
			params.AddMetadata("reason", reason)
		}
	}
	if idempotencyKey != "" {
		params.SetIdempotencyKey(idempotencyKey)
	}
	return refund.New(params)
}

// RefundPaymentIntent fully refunds a PaymentIntent. The idempotency key makes retries return the same refund.
func (s *StripeService) RefundPaymentIntent(paymentIntentID, idempotencyKey string) error {
	_, err := s.Refund(paymentIntentID, 0, "", idempotencyKey)
	return err
}
