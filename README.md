<div align="center">
  <h1>ShopSwift — Microservices Backend</h1>
  <p><strong>Scalable e-commerce backend: Go microservices, Postgres, DynamoDB, Redis, SNS/SQS</strong></p>

  ![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go&logoColor=white)
  ![PostgreSQL](https://img.shields.io/badge/DB-PostgreSQL-336791?logo=postgresql&logoColor=white)
  ![DynamoDB](https://img.shields.io/badge/DB-DynamoDB-4053D6?logo=amazondynamodb&logoColor=white)
  ![Redis](https://img.shields.io/badge/Cache-Redis-DC382D?logo=redis&logoColor=white)
  ![Docker](https://img.shields.io/badge/Deploy-Docker-2496ED?logo=docker&logoColor=white)
  ![LocalStack](https://img.shields.io/badge/Local-AWS-LocalStack-purple)
</div>

---

## Overview

ShopSwift backend is a Go (plus Python agent) microservices platform with an API gateway, BFF, and domain services. Local development uses Docker Compose + **LocalStack** for S3, DynamoDB, SNS, and SQS.

## Architecture (short)

| Layer | Role |
|-------|------|
| **API Gateway** `:8080` | Routing, cookies/auth headers, rate limits, correlation / request IDs |
| **BFF** `:8088` | Frontend aggregation; Redis SetNX checkout idempotency |
| **Domain services** | Auth, user, product, cart, order, payment, inventory, promotion, shipping, notification, agent |
| **Data** | Postgres (transactions), DynamoDB (catalog/inventory), Redis (cart/cache/locks), S3 (images) |
| **Messaging** | SNS topics → SQS queues (checkout, payment, notifications) |

See [MICROSERVICE_ARCHITECTURE.md](MICROSERVICE_ARCHITECTURE.md), [SERVICES_AND_DATABASES.md](SERVICES_AND_DATABASES.md), [SERVICE_ISSUES_AND_AUDIT.md](SERVICE_ISSUES_AND_AUDIT.md), [CLAUDE.md](CLAUDE.md), [backend/docs/architecture.md](backend/docs/architecture.md), [backend/docs/data-and-messaging.md](backend/docs/data-and-messaging.md), and [backend/docs/best-practices-and-gaps.md](backend/docs/best-practices-and-gaps.md).

### Services and ports

| Service | Port | Stack | Primary storage |
|---------|------|-------|-----------------|
| api-gateway | 8080 | Go / Gin | Redis (rate limit) |
| auth-service | 8081 | Go | Postgres |
| product-service | 8082 | Go | DynamoDB + Redis + S3 |
| order-service | 8083 | Go | Postgres + SQS/SNS |
| inventory-service | 8084 | Go | DynamoDB |
| user-service | 8085 | Go | Postgres |
| cart-service | 8086 | Go | Redis |
| payment-service | 8087 | Go | Postgres + Stripe + SQS/SNS |
| bff-service | 8088 | Go | Redis |
| agent-service | 8089→8000 | Python FastAPI | Stateless (calls BFF) |
| promotion-service | 8090 | Go | Postgres + SQS/SNS |
| shipping-service | 8091 | Go | In-memory / JSON rates (no DB) |
| notification-service | 8092 | Go | Postgres + SQS |
| docs (Swagger UI) | 8099 | swagger-ui | — |
| LocalStack | 4566 | AWS emulator | S3/DDB/SNS/SQS |
| postgres | 5432 | Postgres | DB `ecommerce` |
| redis | 6379 | Redis 7 | — |

## Tech stack

- **Languages:** Go 1.25 (workspace), Python (agent-service)
- **HTTP:** Gin; gateway proxies to services and BFF
- **Postgres:** auth, user, order, payment, promotion, notification
- **DynamoDB:** products, categories, inventory (not MongoDB)
- **Redis:** cart, BFF checkout locks, gateway rate limit, product cache
- **AWS (or LocalStack):** S3, SNS, SQS, Secrets Manager (optional), CloudWatch (optional)
- **Payments:** Stripe + stripe-cli webhook forwarding in Compose

## Project structure

```
E-Commerce-backend/
└── backend/
    ├── api-gateway/
    ├── services/
    │   ├── agent-service/          # Python AI agent
    │   ├── auth-service/
    │   ├── bff-service/
    │   ├── cart-service/
    │   ├── common/                 # Shared Go libs
    │   ├── inventory-service/
    │   ├── notification-service/
    │   ├── order-service/
    │   ├── payment-service/
    │   ├── product-service/
    │   ├── promotion-service/
    │   ├── shipping-service/
    │   └── user-service/
    ├── docs/                       # Architecture, OpenAPI, API md
    ├── migrations/                 # SQL migrations (golang-migrate)
    ├── localstack/                 # LocalStack image + bootstrap
    ├── infrastructure/aws/         # Terraform / deploy helpers
    ├── docker-compose.yml
    ├── docker-compose.localstack.yml
    └── scripts/dev-up.sh           # Recommended local start
```

## Getting started

### Prerequisites

- Docker + Docker Compose
- Go 1.25+ (optional, for running services outside Docker)
- Copy `backend/.env.example` → `backend/.env` and set secrets (JWT, Stripe, SMTP, Postgres password)

### Run the full stack (recommended)

Local AWS emulation is **required** for notification, order, payment, and product flows:

```bash
cd backend
cp -n .env.example .env   # if needed
./scripts/dev-up.sh
# equivalent:
# docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d --build
```

Useful URLs:

- Gateway: http://localhost:8080  
- BFF: http://localhost:8088  
- OpenAPI UI: http://localhost:8099  
- LocalStack: http://localhost:4566  

### Migrations

```bash
cd backend
./scripts/migrate.sh up
```

Set `ALLOW_AUTO_MIGRATE=false` in production so schema comes only from SQL migrations. Local Compose defaults `ALLOW_AUTO_MIGRATE=true` for DX.

### Docs

- [Architecture](backend/docs/architecture.md)
- [Data & messaging](backend/docs/data-and-messaging.md)
- [API docs index](backend/docs/api/README.md)
- [Best practices & gaps](backend/docs/best-practices-and-gaps.md)

## Security

- Auth service issues JWT / cookies; gateway forwards identity
- Secrets via env / IAM / LocalStack secrets (not committed)
- Stripe webhooks verified with signing secret; event ID dedup

## Roadmap (remaining)

- [ ] Broader contract / SQS consumer tests in CI
- [ ] Full OpenAPI agent-ready pass (`operationId`, error schemas)
- [ ] Prometheus / OpenTelemetry exporters
- [ ] Remove unused Mongo helpers and dormant shipping DB code

## Author

**Yash Rajoria** — [GitHub](https://github.com/yashrajoria) · [LinkedIn](https://www.linkedin.com/in/yashrajoria)
