.PHONY: help build run stop restart logs clean test

# Default target
help:
	@echo "ADS-B Scope - Development Commands"
	@echo ""
	@echo "Docker Commands:"
	@echo "  make build     - Build Docker images"
	@echo "  make run       - Start all containers"
	@echo "  make stop      - Stop all containers"
	@echo "  make restart   - Restart all containers"
	@echo "  make logs      - View container logs"
	@echo "  make clean     - Stop and remove all containers and volumes"
	@echo ""
	@echo "Local Development:"
	@echo "  make dev       - Build and run locally (requires Go)"
	@echo "  make test      - Run tests"
	@echo "  make fmt       - Format code"
	@echo "  make lint      - Run linter"

# Docker commands
build:
	docker-compose build

run:
	docker-compose up -d
	@echo "Application starting..."
	@echo "Web UI: http://localhost:8080"
	@echo "Database: localhost:5432"

stop:
	docker-compose stop

restart:
	docker-compose restart

logs:
	docker-compose logs -f

clean:
	docker-compose down -v
	rm -rf bin/

# Local development commands
dev:
	go build -o bin/ads-bscope ./cmd/ads-bscope
	./bin/ads-bscope

test:
	go test ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

fmt:
	go fmt ./...

lint:
	golangci-lint run

vet:
	go vet ./...
