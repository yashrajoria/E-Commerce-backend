FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY pkg/common /pkg/common

COPY services/identity-service/go.mod services/identity-service/go.sum ./
RUN go mod download

COPY services/identity-service ./

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o /identity-service .

FROM alpine:3.19

# Add timezone data
RUN apk add --no-cache tzdata
RUN addgroup -S app && adduser -S -G app app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /identity-service /identity-service

USER app
EXPOSE 8081
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8081/health/live || exit 1

ENTRYPOINT ["/identity-service"]
