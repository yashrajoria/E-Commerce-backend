FROM golang:1.25-alpine AS builder
WORKDIR /src

# Copy module files first
COPY backend/services/product-service/go.mod backend/services/product-service/go.sum ./


# Provide local module referenced by replace
COPY backend/pkg/aws /pkg/aws
COPY backend/common /common

RUN go mod download

# Copy service sources and build
COPY backend/services/product-service .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
