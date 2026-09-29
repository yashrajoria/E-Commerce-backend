# API documentation index

Canonical OpenAPI: [`../openapi.yaml`](../openapi.yaml)  
Swagger UI (Compose): http://localhost:8099

## Curated (hand-written)

| Doc | Service |
|-----|---------|
| [BFF.md](./BFF.md) | bff-service `:8088` |
| [Promotion-Service.md](./Promotion-Service.md) | promotion-service `:8090` — gateway `/coupons` |
| [Shipping-Service.md](./Shipping-Service.md) | shipping-service `:8091` — rates only |
| [Notification-Service.md](./Notification-Service.md) | notification-service `:8092` — SQS consumer |

## Generated from OpenAPI

Thin endpoint lists (method + path + summary) live under [`generated/`](./generated/). Regenerate:

```bash
cd backend
npm install js-yaml   # if needed
node scripts/generate-api-md.js docs/openapi.yaml docs/api/generated
```

**Note:** OpenAPI `servers` may omit inventory/notification; use Compose ports from the root README. Notification is under-tagged — see curated Notification doc. Full agent-ready OpenAPI (`operationId`, error schemas) is a P2 follow-up.
