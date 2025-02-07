# Use the prebuilt Go Alpine image for building
FROM golang:1.23.6-alpine3.21 AS builder

# Install necessary dependencies
RUN apk add --no-cache ca-certificates

# Set the working directory for Go modules
WORKDIR /app

# Copy only go.mod and go.sum first (better caching)
COPY gospace/go.mod gospace/go.sum /app/gospace/

# Download dependencies early to leverage Docker layer caching
WORKDIR /app/gospace
RUN go mod tidy

# Copy the rest of the application source
COPY gospace /app/gospace

# Build the application
RUN go build -o gospace ./cmd/api/main.go

# Create the final image
FROM alpine:3.21.2

# Install necessary runtime dependencies
RUN apk add --no-cache ca-certificates

# Create a non-root user for security
RUN addgroup -S goapp && adduser -S goapp -G goapp

# Copy the built Go binary from the builder stage
COPY --from=builder /app/gospace/gospace /usr/local/bin/gospace

RUN mkdir -p /var/run/redis && chown goapp:goapp /var/run/redis

# Switch to non-root user
USER goapp