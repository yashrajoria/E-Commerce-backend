# Multi-service build for ShopSwift (Production / Cloud Deployment)
FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git tzdata ca-certificates

# Copy go workspace files
COPY backend/go.work backend/go.work.sum ./
COPY backend/go.mod backend/go.sum ./

# Copy shared packages
COPY backend/pkg ./pkg

# Copy services and gateway
COPY backend/api-gateway ./api-gateway
COPY backend/services/identity-service ./services/identity-service
COPY backend/services/catalog-service ./services/catalog-service
COPY backend/services/order-service ./services/order-service
COPY backend/services/notification-service ./services/notification-service

# Build all 5 Go services into /out
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/api-gateway ./api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/identity-service ./services/identity-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/catalog-service ./services/catalog-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/order-service ./services/order-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/notification-service ./services/notification-service

# Final lightweight runtime image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata bash curl

WORKDIR /app

# Copy binaries
COPY --from=builder /out/api-gateway /app/api-gateway
COPY --from=builder /out/identity-service /app/identity-service
COPY --from=builder /out/catalog-service /app/catalog-service
COPY --from=builder /out/order-service /app/order-service
COPY --from=builder /out/notification-service /app/notification-service

# Copy email templates
COPY backend/services/notification-service/templates /app/templates

# Copy startup script
COPY start.sh /app/start.sh
RUN chmod +x /app/start.sh /app/api-gateway /app/identity-service /app/catalog-service /app/order-service /app/notification-service

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -f http://127.0.0.1:${PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/start.sh"]
