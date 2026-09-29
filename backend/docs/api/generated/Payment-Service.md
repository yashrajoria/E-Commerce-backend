# Payment Service API

Endpoints:

- **GET** /payment/status/by-order/{order_id} — Get payment status by order ID
- **POST** /payment/verify-payment — Verify payment after Stripe redirect
- **POST** /stripe/webhook — Stripe webhook receiver