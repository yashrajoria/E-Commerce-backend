# ShopSwift End-to-End Demo Script (5–10 Minutes)

This script demonstrates the complete ShopSwift microservices architecture: Authentication, Catalog browsing, Cart management, Inventory reservation, Checkout saga with Stripe, Outbox asynchronous event publishing, and AI Admin Assistant.

---

## Prerequisites
Stack running locally via Docker Compose:
```bash
cd backend
docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d
```

---

## 1. Authentication & Session Setup (1 Minute)
Log in as the standard demo customer:
```bash
curl -c /tmp/cookies.txt -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice.johnson@shopswift-demo.test","password":"Demo123!"}'
```
*Expected response*: `{"message":"Logged in successfully", "user": {"role":"user", ...}}` with `__session` and `refresh_token` HTTP-only cookies stored in `/tmp/cookies.txt`.

---

## 2. Product Browsing & Inventory Check (1 Minute)
Fetch top products from the catalog:
```bash
curl -s http://localhost:8080/products | jq '.products[0:3]'
```
Notice each product features real-time inventory quantity backed by Postgres.

---

## 3. Cart Management (1 Minute)
Add a product to Alice's cart:
```bash
# Add 1 unit of product
curl -b /tmp/cookies.txt -c /tmp/cookies.txt \
  -X POST http://localhost:8080/cart/add \
  -H "Content-Type: application/json" \
  -d '{"items":[{"product_id":"00000000-0000-4000-8000-00000000001d","quantity":1}]}'
```

View updated cart state in Redis:
```bash
curl -b /tmp/cookies.txt http://localhost:8080/cart
```

---

## 4. Checkout Saga & Inventory Reservation (2 Minutes)
Trigger checkout with an idempotency key:
```bash
IDEM_KEY="demo-run-$(date +%s)"
CHECKOUT_RESP=$(curl -s -b /tmp/cookies.txt -c /tmp/cookies.txt \
  -X POST http://localhost:8080/cart/checkout \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEM_KEY" \
  -d '{"currency":"USD"}')

echo "$CHECKOUT_RESP"
ORDER_ID=$(echo "$CHECKOUT_RESP" | jq -r '.order_id')
echo "New Order ID: $ORDER_ID"
```

Verify that inventory was reserved in Postgres (`catalog.stock_reservations`) and the order is in `pending_payment` status:
```bash
curl -s -b /tmp/cookies.txt http://localhost:8080/orders/$ORDER_ID | jq '.order.Status'
```

---

## 5. Stripe Hosted Checkout & Webhook Fulfillment (2 Minutes)
Retrieve the live Stripe Checkout URL generated for the order:
```bash
curl -s -b /tmp/cookies.txt "http://localhost:8080/payment/status/by-order/$ORDER_ID" | jq '.checkout_url'
```

Simulate the customer completing payment using the Stripe CLI container:
```bash
docker exec backend-stripe-cli-1 stripe trigger checkout.session.completed \
  --add checkout_session:metadata.order_id=$ORDER_ID \
  --add checkout_session:metadata.user_id="11111111-1111-4111-8111-111111111101"
```

Verify the order has transitioned to `paid`:
```bash
curl -s -b /tmp/cookies.txt http://localhost:8080/orders/$ORDER_ID | jq '{status: .order.Status, completed_at: .order.CompletedAt}'
```

Verify the notification event was dispatched:
```bash
docker logs backend-notification-service-1 --since=1m 2>&1 | grep "order_confirmed"
```

---

## 6. Outbox Reliability & Fault-Tolerance (1 Minute)
Inspect the transactional outbox table:
```bash
docker exec backend-postgres-1 psql -U postgres -d ecommerce -c \
  "SELECT id, event_type, status, destination_type, published_at FROM outbox_events ORDER BY created_at DESC LIMIT 3;"
```
Both `payment_request` and `order_created` events were committed atomically with the order and published to AWS SQS / SNS.

---

## 7. AI Admin Assistant & Safeguarded Mutation (2 Minutes)
Log in as the store administrator:
```bash
curl -c /tmp/admin_cookies.txt -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"merchant.owner@shopswift-demo.test","password":"Demo123!"}'
```

Query store analytics through the agent:
```bash
curl -s -b /tmp/admin_cookies.txt -X POST http://localhost:8080/agent/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"What are our top selling products and current stock levels?"}'
```

Test safety boundary on mutation:
```bash
# Requesting an order cancellation requires two-phase confirmation:
curl -s -b /tmp/admin_cookies.txt -X POST http://localhost:8080/agent/chat \
  -H "Content-Type: application/json" \
  -d "{\"message\":\"Please cancel order $ORDER_ID because customer requested refund\"}"
```
The response returns `requires_confirmation: true` with a pending request ID without executing any destructive operation until confirmed by the administrator.
