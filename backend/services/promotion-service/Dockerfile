FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY pkg/aws /pkg/aws
COPY services/common /common

COPY services/promotion-service/go.mod services/promotion-service/go.sum ./
RUN go mod download

COPY services/promotion-service ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /promotion-service .

FROM alpine:3.19

# Add timezone data
RUN apk add --no-cache tzdata
RUN addgroup -S app && adduser -S -G app app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /promotion-service /promotion-service

USER app
# NOTE: was mislabeled 8092 (notification-service's port) — promotion-service
# actually listens on 8090 (see config.go default and docker-compose PORT=8090).
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8090/health/live || exit 1

ENTRYPOINT ["/promotion-service"]