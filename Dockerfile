# Multi-service build for ShopSwift (Production / Cloud Deployment)
FROM golang:1.25-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git tzdata ca-certificates

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

# Runtime stage uses official Amazon DynamoDB Local image (Java + DynamoDB pre-installed)
FROM amazon/dynamodb-local:latest

USER root

WORKDIR /app

# Copy compiled Go binaries
COPY --from=builder /out/ /app/

# Copy email templates
COPY backend/services/notification-service/templates /app/templates

# Copy startup script
COPY start.sh /app/start.sh
RUN chmod +x /app/start.sh /app/*

EXPOSE 8080

ENTRYPOINT ["/app/start.sh"]
