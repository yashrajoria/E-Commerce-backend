FROM golang:1.25-alpine AS builder
WORKDIR /src

# Copy module files first (use repo-root paths because CI builds from repo root)
COPY backend/services/auth-service/go.mod backend/services/auth-service/go.sum ./


# Provide local module referenced by replace directive
COPY backend/pkg/aws /pkg/aws
COPY backend/common /common

RUN go mod download

# Copy service sources and build
COPY backend/services/auth-service .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
