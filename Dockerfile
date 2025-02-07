# Use the prebuilt Go Alpine image
FROM golang:1.23.6-alpine3.21 AS builder

# Install necessary dependencies
RUN apk add --no-cache ca-certificates

# Set the working directory
WORKDIR /app

# Copy the local gospace directory into the container
COPY gospace /app

# Download dependencies
RUN go mod tidy

# Build the application
RUN go build -o gospace ./cmd/api/main.go

# Create the final image
FROM alpine:3.21.2

# Install necessary runtime dependencies
RUN apk add --no-cache ca-certificates

# Create a non-root user for security
RUN addgroup -S goapp && adduser -S goapp -G goapp

# Copy the built Go binary from the builder stage
COPY --from=builder /app/gospace /usr/local/bin/gospace

# Change ownership of the Redis socket directory (shared volume)
RUN mkdir -p /var/run/redis && chown goapp:goapp /var/run/redis

# Switch to non-root user
USER goapp

# Set the default port to 6060
ENV APP_PORT=6060

# Expose the internal Go application port (to be mapped in docker-compose)
EXPOSE 6060

# Run the Go application with a dynamic port
ENTRYPOINT ["/usr/local/bin/gospace"]
