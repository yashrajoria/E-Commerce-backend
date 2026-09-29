# API documentation index

Canonical OpenAPI: [`../openapi.yaml`](../openapi.yaml)  
Swagger UI (Compose): http://localhost:8099

## Curated (hand-written)

| Doc | Service |
|-----|---------|
| [BFF.md](./BFF.md) | retained `/bff/admin/*` contract + removed storefront paths |
| [Promotion-Service.md](./Promotion-Service.md) | order-service `:8083` — gateway `/coupons` (merged in-process) |
| [Shipping-Service.md](./Shipping-Service.md) | order-service `:8083` — rates only (merged in-process) |
| [Notification-Service.md](./Notification-Service.md) | notification-service `:8092` — SQS consumer |

## Generated from OpenAPI

Thin endpoint lists (method + path + summary) live under [`generated/`](./generated/). Regenerate:

```bash
cd backend
npm install js-yaml   # if needed
node scripts/generate-api-md.js docs/openapi.yaml docs/api/generated
```

**Note:** OpenAPI `servers` may omit inventory/notification; use Compose ports from the root README. Notification is under-tagged — see curated Notification doc. Full agent-ready OpenAPI (`operationId`, error schemas) is a P2 follow-up.
