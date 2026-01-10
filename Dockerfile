# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum (if exists) first to leverage Docker cache
COPY go.mod ./
COPY go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ads-bscope ./cmd/ads-bscope

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests and wget for health checks
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 adsbscope && \
    adduser -D -u 1000 -G adsbscope adsbscope

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/ads-bscope .

# Copy configuration files
COPY --from=builder /build/configs ./configs

# Change ownership
RUN chown -R adsbscope:adsbscope /app

# Switch to non-root user
USER adsbscope

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./ads-bscope"]
