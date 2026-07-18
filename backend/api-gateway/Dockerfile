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

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /api-gateway /api-gateway

EXPOSE 8080
ENTRYPOINT ["/api-gateway"]