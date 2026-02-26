FROM golang:1.25-alpine

WORKDIR /app

RUN apk add --no-cache git

RUN go install github.com/cosmtrek/air@v1.49.0

COPY services/promotion-service/go.mod services/promotion-service/go.sum ./

# Ensure local packages referenced by replace directives are available in the build context
COPY pkg/aws /pkg/aws
COPY services/common /common

RUN go mod download

COPY services/promotion-service .
RUN mkdir -p /app/tmp

EXPOSE 8092
CMD ["air", "-c", ".air.toml"]
