# Notification Service API

Base URL: `http://localhost:8092` (service internal)  
Gateway: admin/log routes under gateway (see OpenAPI / BFF admin). Primary work is **SQS consumption**, not public REST.

## Runtime behavior

- Consumes `NOTIFICATION_SQS_QUEUE_URL` / `SQS_QUEUE_URL` (LocalStack: `notification-queue`).
- Subscribed via SNS topic `notification-events` (auth emails, order/payment notifications, etc.).
- Persists rows to Postgres `notification_logs` and sends email via SMTP when configured.

## HTTP

- **GET /health** — liveness alias
- **GET /health/live** — process up
- **GET /health/ready** — Postgres (and queue config) ready
- **GET /notifications/log** — notification log listing (auth/admin as wired by gateway)

OpenAPI currently under-tags this service; prefer this doc + Compose env for queue URLs.
