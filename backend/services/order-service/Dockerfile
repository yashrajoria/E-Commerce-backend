FROM golang:1.25-alpine AS builder
WORKDIR /src

# Copy go.mod first
COPY services/order-service/go.mod ./

# Copy go.sum
COPY services/order-service/go.sum ./


# Ensure local replace path is present for go mod download (monorepo local module)
# The go.mod in this repo uses: replace github.com/yashrajoria/E-Commerce-backend/backend/pkg/aws => ../../pkg/aws
# That resolves inside the container to /pkg/aws when working dir is /src, so provide it.
COPY pkg/aws /pkg/aws
COPY services/common /common

RUN go mod download

# Copy service sources and build
COPY services/order-service .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]