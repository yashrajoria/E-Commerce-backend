FROM golang:1.25-alpine

WORKDIR /app

RUN apk add --no-cache git

RUN go install github.com/cosmtrek/air@v1.49.0

COPY services/product-service/go.mod services/product-service/go.sum ./

# Provide local module referenced by replace
COPY pkg/aws /pkg/aws
COPY services/common /common

RUN go mod download

COPY services/product-service .
RUN mkdir -p /app/tmp

EXPOSE 8082
CMD ["air", "-c", ".air.toml"]
