# Build stage
FROM golang:1.21-alpine AS builder

# Install necessary packages
RUN apk --no-cache add ca-certificates git

# Set working directory
WORKDIR /app

# Set Go proxy and checksum database
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org

# Copy go mod and sum files first
COPY go.mod go.sum ./

# Clean and download dependencies with retry
RUN go clean -modcache && \
    go mod download && \
    go mod verify

# Copy the source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o main .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN adduser -D -s /bin/sh appuser

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Change ownership to non-root user
RUN chown appuser:appuser /app/main

# Switch to non-root user
USER appuser

# Expose port (Railway will provide the PORT env var)
EXPOSE 8080

# Command to run the executable
CMD ["./main"]