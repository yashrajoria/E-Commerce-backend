FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY pkg/aws /pkg/aws
COPY services/common /common

COPY services/cart-service/go.mod services/cart-service/go.sum ./
RUN go mod download

COPY services/cart-service ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /cart-service .

FROM alpine:3.19

# Add timezone data
RUN apk add --no-cache tzdata

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /cart-service /cart-service

EXPOSE 8086
ENTRYPOINT ["/cart-service"]