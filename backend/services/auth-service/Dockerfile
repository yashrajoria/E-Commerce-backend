FROM golang:1.25-alpine AS builder
WORKDIR /src

# Copy module files first (use repo-root paths because CI builds from repo root)
COPY services/auth-service/go.mod services/auth-service/go.sum ./



# Provide local module referenced by replace directive
COPY pkg/aws /pkg/aws
COPY services/common /common

RUN go mod download

# Copy service sources and build
COPY services/auth-service .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
