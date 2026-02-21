FROM golang:1.25-alpine AS builder
WORKDIR /src

# Copy module files first
COPY services/inventory-service/go.mod services/inventory-service/go.sum ./



# Provide local module referenced by replace
COPY pkg/aws /pkg/aws
COPY services/common /common

RUN go mod download

# Copy service sources and build
COPY services/inventory-service .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
