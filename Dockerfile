# Multi-service build for ShopSwift (Production / Cloud Deployment)
FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git tzdata ca-certificates wget tar

# Copy go workspace files
COPY backend/go.work backend/go.work.sum ./
COPY backend/go.mod backend/go.sum ./

# Copy shared packages
COPY backend/pkg ./pkg

# Copy services, tools, and gateway
COPY backend/api-gateway ./api-gateway
COPY backend/services ./services
COPY backend/tools ./tools

# Build all 5 Go services + init-dynamo tool into /out
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/api-gateway ./api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/identity-service ./services/identity-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/catalog-service ./services/catalog-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/order-service ./services/order-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/notification-service ./services/notification-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /out/init-dynamo ./tools/init-dynamo

# Download official AWS DynamoDB Local for embedded catalog storage
WORKDIR /opt/dynamodb
RUN wget -q https://d1niqa97ap6et874.cloudfront.net/dynamodb_local_latest.tar.gz && \
    tar -xzf dynamodb_local_latest.tar.gz && \
    rm dynamodb_local_latest.tar.gz

# Final lightweight runtime image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata bash curl openjdk17-jre-headless

WORKDIR /app

# Copy DynamoDB Local
COPY --from=builder /opt/dynamodb /opt/dynamodb

# Copy compiled binaries
COPY --from=builder /out/ /app/

# Copy email templates
COPY backend/services/notification-service/templates /app/templates

# Copy startup script
COPY start.sh /app/start.sh
RUN chmod +x /app/start.sh /app/*

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -f http://127.0.0.1:${PORT:-8080}/health || exit 1

ENTRYPOINT ["/app/start.sh"]
