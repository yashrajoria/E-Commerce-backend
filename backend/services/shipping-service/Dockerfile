FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY services/shipping-service/go.mod services/shipping-service/go.sum ./
RUN go mod download

COPY services/shipping-service ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /shipping-service .

FROM alpine:3.19

# Add timezone data
RUN apk add --no-cache tzdata

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /shipping-service /shipping-service

EXPOSE 8091
ENTRYPOINT ["/shipping-service"]