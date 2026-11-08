# 📖 ShopSwift API Documentation Index

Entry point for the ShopSwift backend API docs. The canonical contract is the OpenAPI spec in `backend/docs/openapi.yaml`.

---

## 🚀 Interactive Exploration

- **Canonical OpenAPI 3.0 spec**: [`../openapi.yaml`](../openapi.yaml)
- **Edge gateway**: `http://localhost:8080`

---

## 📚 Curated Service Docs

| Document | Service | Scope |
|:---|:---|:---|
| [Notification-Service.md](./Notification-Service.md) | `notification-service` (`:8092`) | Email templates, SQS consumer, delivery logs, DLQ handling |

Other sections in the repo: `../architecture.md`, `../data-and-messaging.md`, `../adr/`, `../backup-restore-runbook.md`.

---

## 📑 Core Endpoints Index

### Store Credit (`order-service`, `:8083`)
- `GET /store-credit/balance`: available and held balance.
- `GET /store-credit/ledger`: ledger history.
- `POST /store-credit/grants`: admin credit grants with reason.
- `POST /store-credit/adjustments`: admin balance adjustments.
- `GET /store-credit/reconcile`: reconcile ledger against balances.

### Refunds & Stripe (`order-service`, `:8083`)
- `POST /orders/:id/refund`: full or partial refund. Mesh token required. Restocks items.
- `POST /stripe/webhook`: signature-verified webhook with event dedup.
- `POST /payment/create-checkout`: Stripe Hosted Checkout session.

### Promotions, Coupons & Shipping (`order-service`, `:8083`)
- `POST /promotions/optimize-basket`: basket optimizer.
- `POST /coupons/validate`: coupon validation.
- `POST /shipping/rates`: zone shipping rates.

### Flash Sale Waiting Room (`catalog-service`, `:8082`)
- `POST /inventory/flash-sale/enter`: join the waiting room.
- `GET /inventory/flash-sale/status`: queue position.
- `POST /inventory/flash-sale/claim`: claim a purchase slot.
- `POST /inventory/flash-sale/configure`: admin slot count and lease TTL.

### AI Shopper & Admin Assistant (`agent-service`, `:8089`)
- `POST /shopper/query`: shopper query (gateway maps this to `/agent/query`).
- `POST /shopper/query/stream`: shopper query with SSE streaming.
- `POST /agent/query`: admin assistant query.

---

## 🔄 Regenerating API Markdown

`scripts/generate-api-md.js` writes Markdown from the OpenAPI spec. Its output folder `docs/api/generated/` is not in the repo yet, so run the script to create it:

```bash
cd backend
npm install js-yaml   # if needed
node scripts/generate-api-md.js docs/openapi.yaml docs/api/generated
```
