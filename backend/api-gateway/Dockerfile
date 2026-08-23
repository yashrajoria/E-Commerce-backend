FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git
RUN apk add --no-cache tzdata

# Copy local replaced packages first.
# go.mod replaces: ../pkg/aws and ../services/common (relative to WORKDIR /app)
COPY pkg/aws /pkg/aws
COPY services/common /services/common

COPY api-gateway/go.mod api-gateway/go.sum ./
RUN go mod download

COPY api-gateway ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /api-gateway .

FROM alpine:3.19

RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /api-gateway /api-gateway

USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/api-gateway"]