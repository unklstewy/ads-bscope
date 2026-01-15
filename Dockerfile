# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./

# Download dependencies (cached if go.mod/go.sum unchanged)
RUN go mod download
RUN go mod verify

# Copy source code
COPY . .

# Build flags for optimized static binaries
# -w: omit DWARF symbol table
# -s: omit symbol table and debug information
# -a: force rebuilding of packages
ARG LDFLAGS="-w -s -extldflags '-static'"

# Build all binaries (CGO_ENABLED=0 ensures static linking)
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="${LDFLAGS}" -o collector ./cmd/collector
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="${LDFLAGS}" -o web-server ./cmd/web-server
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="${LDFLAGS}" -o fetch-flightplans ./cmd/fetch-flightplans
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="${LDFLAGS}" -o verify-nasr ./cmd/verify-nasr
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="${LDFLAGS}" -o verify-flightplans ./cmd/verify-flightplans

# Runtime stage for collector service
FROM scratch AS collector

# Copy ca-certificates, timezone data, and passwd for non-root user
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Set working directory
WORKDIR /app

# Copy binary and configs
COPY --from=builder /build/collector /app/
COPY --from=builder /build/configs /app/configs

# Use non-root user (UID 65534 = nobody)
USER 65534:65534

# Expose port
EXPOSE 8080

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["./collector", "--health-check"] || exit 1

# Run the collector service
CMD ["./collector"]

# Runtime stage for fetch-flightplans
FROM scratch AS fetch-flightplans

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

WORKDIR /app

COPY --from=builder /build/fetch-flightplans /app/
COPY --from=builder /build/configs /app/configs

USER 65534:65534

CMD ["./fetch-flightplans"]

# Runtime stage for verify-nasr
FROM scratch AS verify-nasr

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

WORKDIR /app

COPY --from=builder /build/verify-nasr /app/
COPY --from=builder /build/configs /app/configs

USER 65534:65534

CMD ["./verify-nasr"]

# Runtime stage for verify-flightplans
FROM scratch AS verify-flightplans

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

WORKDIR /app

COPY --from=builder /build/verify-flightplans /app/
COPY --from=builder /build/configs /app/configs

USER 65534:65534

CMD ["./verify-flightplans"]

# Runtime stage for web-server (PWA + REST API)
FROM alpine:latest AS web-server

# Install wget for healthcheck
RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

# Copy binary, configs, and web static files
COPY --from=builder /build/web-server /app/
COPY --from=builder /build/configs /app/configs
COPY --from=builder /build/web/static /app/web/static

# Use non-root user
USER 65534:65534

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/system/status || exit 1

# Run the web server
CMD ["./web-server", "--port", "8080"]

# Default runtime stage (backwards compatible)
FROM collector AS default
