.PHONY: help build run stop restart logs clean test fmt lint vet \
        test-coverage test-unit test-integration test-all coverage-report \
        build-collector build-fetch-flightplans build-verify-nasr build-verify-flightplans \
        build-all clean-all dev dev-collector check pre-commit

# Build configuration
GO := go
GOFLAGS := -v
LDFLAGS := -w -s
BINARY_DIR := bin
COVERAGE_DIR := coverage

# Binaries
COLLECTOR_BIN := $(BINARY_DIR)/collector
FETCH_FLIGHTPLANS_BIN := $(BINARY_DIR)/fetch-flightplans
VERIFY_NASR_BIN := $(BINARY_DIR)/verify-nasr
VERIFY_FLIGHTPLANS_BIN := $(BINARY_DIR)/verify-flightplans
ADS_BSCOPE_BIN := $(BINARY_DIR)/ads-bscope

# Default target
help:
	@echo "ADS-B Scope - Development Commands"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make build              - Build Docker images"
	@echo "  make run                - Start all containers (with health checks)"
	@echo "  make stop               - Stop all containers"
	@echo "  make restart            - Restart all containers"
	@echo "  make logs               - View container logs"
	@echo "  make clean              - Stop and remove all containers and volumes"
	@echo ""
	@echo "Local Build:"
	@echo "  make build-all          - Build all binaries"
	@echo "  make build-collector    - Build collector service"
	@echo "  make build-fetch-flightplans - Build flightplan fetcher"
	@echo "  make build-verify-nasr  - Build NASR verifier"
	@echo "  make build-verify-flightplans - Build flightplan verifier"
	@echo ""
	@echo "Local Development:"
	@echo "  make dev                - Build and run ads-bscope locally"
	@echo "  make dev-collector      - Build and run collector service locally"
	@echo ""
	@echo "Testing:"
	@echo "  make test               - Run all tests"
	@echo "  make test-unit          - Run unit tests only"
	@echo "  make test-integration   - Run integration tests (requires DB)"
	@echo "  make test-coverage      - Run tests with coverage report"
	@echo "  make coverage-report    - Generate HTML coverage report"
	@echo ""
	@echo "Code Quality:"
	@echo "  make fmt                - Format code"
	@echo "  make lint               - Run linter (golangci-lint)"
	@echo "  make vet                - Run go vet"
	@echo "  make check              - Run fmt + vet + lint"
	@echo "  make pre-commit         - Run all checks + tests before commit"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean-all          - Remove all build artifacts and coverage reports"

# Docker commands
build:
	docker-compose build

run:
	docker-compose up -d
	@echo "Application starting..."
	@echo "Waiting for health checks..."
	@sleep 5
	@docker-compose ps
	@echo ""
	@echo "Web UI: http://localhost:8080"
	@echo "Database: localhost:5432"
	@echo ""
	@echo "Use 'make logs' to view logs"

stop:
	docker-compose stop

restart:
	docker-compose restart

logs:
	docker-compose logs -f

clean:
	docker-compose down -v
	rm -rf $(BINARY_DIR)/

# Build commands
$(BINARY_DIR):
	mkdir -p $(BINARY_DIR)

$(COVERAGE_DIR):
	mkdir -p $(COVERAGE_DIR)

build-collector: $(BINARY_DIR)
	@echo "Building collector service..."
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(COLLECTOR_BIN) ./cmd/collector
	@echo "✓ Built: $(COLLECTOR_BIN)"

build-fetch-flightplans: $(BINARY_DIR)
	@echo "Building fetch-flightplans..."
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(FETCH_FLIGHTPLANS_BIN) ./cmd/fetch-flightplans
	@echo "✓ Built: $(FETCH_FLIGHTPLANS_BIN)"

build-verify-nasr: $(BINARY_DIR)
	@echo "Building verify-nasr..."
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(VERIFY_NASR_BIN) ./cmd/verify-nasr
	@echo "✓ Built: $(VERIFY_NASR_BIN)"

build-verify-flightplans: $(BINARY_DIR)
	@echo "Building verify-flightplans..."
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(VERIFY_FLIGHTPLANS_BIN) ./cmd/verify-flightplans
	@echo "✓ Built: $(VERIFY_FLIGHTPLANS_BIN)"

build-all: build-collector build-fetch-flightplans build-verify-nasr build-verify-flightplans
	@echo ""
	@echo "✓ All binaries built successfully"
	@ls -lh $(BINARY_DIR)/

# Local development commands
dev: $(BINARY_DIR)
	@echo "Building ads-bscope..."
	$(GO) build $(GOFLAGS) -o $(ADS_BSCOPE_BIN) ./cmd/ads-bscope
	@echo "Starting ads-bscope..."
	./$(ADS_BSCOPE_BIN)

dev-collector: build-collector
	@echo "Starting collector service..."
	./$(COLLECTOR_BIN)

# Testing commands
test:
	@echo "Running all tests..."
	$(GO) test $(GOFLAGS) ./...

test-unit:
	@echo "Running unit tests..."
	$(GO) test $(GOFLAGS) -short ./...

test-integration:
	@echo "Running integration tests (requires PostgreSQL)..."
	@echo "Note: Ensure database is running (make run or local PostgreSQL)"
	$(GO) test $(GOFLAGS) -run Integration ./...

test-all: test

test-coverage: $(COVERAGE_DIR)
	@echo "Running tests with coverage..."
	$(GO) test $(GOFLAGS) -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	@echo ""
	@echo "Coverage summary:"
	@$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out | grep total
	@echo ""
	@echo "Generating detailed coverage reports..."
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/pkg-config.out ./pkg/config
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/pkg-adsb.out ./pkg/adsb
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/pkg-tracking.out ./pkg/tracking
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/internal-db.out ./internal/db
	@echo ""
	@echo "Package coverage:"
	@echo "  pkg/config:   $$($(GO) tool cover -func=$(COVERAGE_DIR)/pkg-config.out | grep total | awk '{print $$3}')"
	@echo "  pkg/adsb:     $$($(GO) tool cover -func=$(COVERAGE_DIR)/pkg-adsb.out | grep total | awk '{print $$3}')"
	@echo "  pkg/tracking: $$($(GO) tool cover -func=$(COVERAGE_DIR)/pkg-tracking.out | grep total | awk '{print $$3}')"
	@echo "  internal/db:  $$($(GO) tool cover -func=$(COVERAGE_DIR)/internal-db.out | grep total | awk '{print $$3}')"
	@echo ""
	@echo "Run 'make coverage-report' to view HTML report"

coverage-report: test-coverage
	@echo "Opening HTML coverage report..."
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out

# Code quality commands
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	@echo "✓ Code formatted"

lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "Error: golangci-lint not installed. Install: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run
	@echo "✓ Linter passed"

vet:
	@echo "Running go vet..."
	$(GO) vet ./...
	@echo "✓ Vet passed"

check: fmt vet lint
	@echo ""
	@echo "✓ All quality checks passed"

pre-commit: check test
	@echo ""
	@echo "✓ Pre-commit checks complete - ready to commit!"

# Cleanup commands
clean-all:
	@echo "Cleaning build artifacts..."
	rm -rf $(BINARY_DIR)/ $(COVERAGE_DIR)/
	@echo "✓ Cleanup complete"
