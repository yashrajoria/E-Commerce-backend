FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY pkg/aws /pkg/aws
COPY services/common /common

COPY services/notification-service/go.mod services/notification-service/go.sum ./
COPY services/notification-service/templates /templates

RUN go mod download

COPY services/notification-service ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /notification-service .

FROM alpine:3.19

# Add timezone data
RUN apk add --no-cache tzdata

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /notification-service /notification-service
COPY --from=builder /templates /app/templates

EXPOSE 8092
ENTRYPOINT ["/notification-service"]