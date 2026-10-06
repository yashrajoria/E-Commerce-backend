# CLAUDE.md — ShopSwift Microservices Backend

This document contains key commands, architectural rules, code style guidelines, and service directory mappings for Claude (and Claude Code CLI) working on the ShopSwift e-commerce microservices repository.

---

## 1. Project Summary

- **Repository**: ShopSwift E-Commerce Microservices Backend
- **Core Stack**: Go 1.25 (multi-module workspace in `backend/go.work`), Python 3.11+ FastAPI (`agent-service`), PostgreSQL (`ecommerce` DB), DynamoDB (LocalStack / AWS), Redis 7, AWS S3, SNS/SQS.
- **Architecture**: Edge API Gateway (`:8080`) + 3 Domain Microservices (identity, catalog, order) + notification consumer + 1 AI Agent Service.
- **Documentation**:
  - [MICROSERVICE_ARCHITECTURE.md](MICROSERVICE_ARCHITECTURE.md) — Mermaid sequence diagrams and storage ownership rules.
  - [SERVICES_AND_DATABASES.md](SERVICES_AND_DATABASES.md) — per-service data model detail.
  - [SERVICE_ISSUES_AND_AUDIT.md](SERVICE_ISSUES_AND_AUDIT.md) — known bugs, security gaps, and perf issues per service (check before touching a service to avoid re-introducing a known-flagged pattern).
  - [backend/CLAUDE.md](backend/CLAUDE.md) — directory layout when working directly inside `backend/`.

---

## 2. Service & Port Matrix

| Service | Port | Technology | Primary Data Store | Main Path / Purpose |
|:---|:---:|:---|:---|:---|
| **api-gateway** | `8080` | Go / Gin | Redis | `/` (Edge router, JWT auth check, rate limiting, request correlation) |
| **identity-service** | `8081` | Go / Gin | Postgres (`users`, `refresh_tokens`, `addresses`) | `/auth`, `/users` (Identity, login, token refresh, admin bootstrap, profiles, addresses) |
| **catalog-service** | `8082` | Go / Gin | DynamoDB + Redis + S3 | `/products`, `/categories`, `/inventory`, `/cart` (Catalog, stock, cart, caching) |
| **order-service** | `8083` | Go / Gin | Postgres (`orders`, `order_items`, `coupons`, `payments`, `stripe_processed_events`) + SQS + Stripe | `/orders`, `/coupons`, `/shipping`, `/payment` (Orders, coupons, shipping rates, Stripe) |
| **agent-service** | `8089` | Python / FastAPI | Stateless | `/agent` (AI Assistant via gateway) |
| **notification-service**| `8092` | Go / Gin | Postgres (`notification_logs`) + SQS | `/notifications` (Async notification consumer & email logger) |
| **OpenAPI Docs UI** | `8099` | Swagger UI | — | http://localhost:8099 |
| **LocalStack** | `4566` | AWS Emulator | S3, DDB, SNS, SQS | http://localhost:4566 |
| **PostgreSQL** | `5432` | Postgres 15+ | `ecommerce` DB | Local relational store |
| **Redis** | `6379` | Redis 7 | — | Caching, locks, rate limits |

---

## 3. Common Developer Commands

All commands are run from the `backend/` directory unless specified otherwise.

### 3.1 Environment & Local Cluster
```bash
# Start full local stack with Docker Compose + LocalStack (Recommended)
cd backend
./scripts/dev-up.sh
# Equivalent to:
# docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d --build

# Stop local stack
docker compose -f docker-compose.yml -f docker-compose.localstack.yml down
```

### 3.2 Database Migrations
```bash
cd backend
# Run pending SQL migrations (golang-migrate)
./scripts/migrate.sh up

# Rollback last migration step
./scripts/migrate.sh down 1
```

### 3.3 Seeding Demo Data
```bash
cd backend
# Seed catalog products, categories, product-category links, and inventory stock (DynamoDB + S3)
./scripts/seed_demo_data.sh

# Sync specific parts only
./scripts/seed_demo_data.sh --sync-inventory-only
./scripts/seed_demo_data.sh --sync-category-links-only

# Seed users, addresses, orders, order_items, payments, stripe_processed_events,
# coupons, notification_logs, shipments (Postgres). Run after seed_demo_data.sh
# so order_items can reference real product IDs. Idempotent (wipes prior demo
# rows first). All demo users share password Demo123!; merchant.owner@shopswift-demo.test is role=admin.
./scripts/seed_postgres_data.sh
```

### 3.4 Running Tests
```bash
cd backend

# Run unit tests across all Go workspace modules
go test ./...

# Run unit tests in a specific service (e.g. catalog-service)
cd services/catalog-service && go test -v ./...

# Run a single Go test by name
cd services/catalog-service && go test -v -run TestFuncName ./...

# Run unit tests in Python agent service
cd services/agent-service && pytest

# Run a single Python test
cd services/agent-service && pytest tests/test_file.py::test_name
```

### 3.5 API Specification & Linting
```bash
cd backend

# Lint OpenAPI specification with Spectral
npx @stoplight/spectral-cli lint docs/openapi.yaml
```

---

## 4. Code Style & Architecture Guidelines

### 4.1 Go Workspace & Module Guidelines
- The Go backend uses a Go Workspace (`backend/go.work`). When adding a new Go package or service, add its folder path to `backend/go.work`.
- Keep dependencies lean and prefer standard library or standard ecosystem packages (`gin-gonic/gin`, `gorm.io/gorm`, `aws/aws-sdk-go-v2`, `redis/go-redis/v9`).

### 4.2 Layering & Package Structure
Each Go domain microservice follows standard clean architecture boundaries:
- `main.go`: Entry point, env config loading, logger init, DB setup, route setup, server listen.
- `handlers/` or `controllers/`: Gin HTTP handler functions binding requests and returning JSON responses.
- `services/`: Business logic implementations.
- `repository/` or `models/`: Data access layer (GORM DB calls or AWS SDK v2 DynamoDB queries).
- `routes/`: Gin route group registrations.

### 4.3 Standard Error Handling
- Use `pkg/common/errors` middleware for consistent API error responses.
- Always return standard structured JSON error responses with proper HTTP status codes (`400`, `401`, `403`, `404`, `409`, `500`).

### 4.4 Auth & Header Injection Rules
- **API Gateway (`api-gateway`)** is responsible for validating JWT access tokens.
- **Header Sanitization**: Gateway MUST strip any incoming client headers matching `X-User-ID`, `X-User-Role`, or `X-User-Email` before forwarding to downstream microservices.
- Gateway injects verified `X-User-ID` and `X-User-Role` headers into upstream service requests.
- Microservices retrieve user identity from incoming `X-User-*` HTTP headers set by Gateway.
- Admin endpoints must check `X-User-Role == "admin"`.

### 4.5 Database Ownership Rules
- **PostgreSQL**: Do NOT perform cross-service database operations directly unless specified. Respect service table ownership:
  - `identity-service`: `users`, `refresh_tokens`, `addresses`
  - `order-service`: `orders`, `order_items`, `coupons`, `payments`, `stripe_processed_events`, `payment_outbox_events`
  - `notification-service`: `notification_logs`
- **DynamoDB**:
  - `catalog-service` owns `Products`, `Categories`, `ProductCategories`, and `Inventory` tables. Use `ClientRequestToken` on conditional updates for stock reservations.
- **Redis**:
  - `catalog-service` uses `cart:user:{user_id}` + `idem:cart:*` keys (cart + checkout replay), `product:detail:*` + `products:version` (cache), `bulk_import:*` (import jobs).
  - `api-gateway` uses Redis for request rate limiting.

### 4.6 Asynchronous Messaging & Idempotency Rules
- **SNS/SQS**:
  - `catalog-service` (cart) publishes `checkout.requested` to `order-events` topic.
  - `order-service` consumes `order-processing-queue`, runs checkout → Stripe → webhook fulfillment in-process, and publishes `payment.succeeded` to `payment-events` topic via transactional outbox.
  - `notification-service` consumes `notification-queue`.
- **Idempotency**:
  - Every checkout call must supply an `Idempotency-Key` header.
  - SQS message handlers must handle duplicate deliveries gracefully using `idempotency_key` columns or status checks.
  - Stripe webhooks must verify event signatures and deduplicate using `stripe_processed_events` table before processing.

---

## 5. Key Environment Variables

Core variables configured in `backend/.env`:
```bash
# General
ENV=development
ALLOW_AUTO_MIGRATE=true
GIN_MODE=debug

# Database & Cache
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres
POSTGRES_DB=ecommerce
REDIS_HOST=redis
REDIS_PORT=6379

# LocalStack AWS Configuration
USE_LOCALSTACK=true
LOCALSTACK_ENDPOINT=http://localstack:4566
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_S3_BUCKET=shopswift

# DynamoDB Tables
DDB_TABLE_PRODUCTS=Products
DDB_TABLE_CATEGORIES=Categories
DDB_TABLE_PRODUCT_CATEGORIES=ProductCategories
DDB_TABLE_INVENTORY=Inventory

# Auth & Admin
JWT_SECRET=your_jwt_secret_key
ADMIN_EMAIL=admin@shopswift.com
ADMIN_PASSWORD=AdminPassword123!
```
