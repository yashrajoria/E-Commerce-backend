---
description: Start the full ShopSwift local microservices stack via Docker Compose + LocalStack
---

Start the local development stack:

1. Change directory to `backend/`.
2. Run `./scripts/dev-up.sh` (or `docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d --build`).
3. Verify that the API Gateway (`http://localhost:8080/health`), BFF (`http://localhost:8088/health`), and LocalStack (`http://localhost:4566`) are healthy.
4. Report service status.
