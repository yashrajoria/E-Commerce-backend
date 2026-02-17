FROM golang:1.25-alpine AS builder
WORKDIR /src

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o app ./...

FROM gcr.io/distroless/static-debian11
COPY --from=builder /src/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
