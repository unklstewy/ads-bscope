# Docker Deployment Guide

This document provides comprehensive instructions for building and deploying ads-bscope using Docker.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Services](#services)
- [Development Workflow](#development-workflow)
- [Production Deployment](#production-deployment)
- [Troubleshooting](#troubleshooting)

## Overview

The ads-bscope application uses Docker multi-stage builds to create optimized, secure container images. The architecture includes:

- **PostgreSQL Database**: Persistent storage for aircraft data
- **Collector Service**: Main ADS-B data collection service
- **Fetch Flightplans**: Periodic flightplan retrieval service
- **Verification Tools**: NASR and flightplan verification utilities

### Architecture Benefits

- **Multi-stage builds**: Separate build and runtime stages for minimal image size
- **Scratch-based images**: Final images built from `scratch` for maximum security
- **Static binaries**: No dependencies, fully self-contained applications
- **Health checks**: Automatic service monitoring and restart
- **Resource limits**: Memory and CPU constraints for stability

## Prerequisites

- Docker Engine 20.10+ or Docker Desktop
- Docker Compose v2.0+
- At least 1GB free disk space
- Network access for pulling base images

## Quick Start

### 1. Clone and Configure

```bash
# Clone repository
git clone https://github.com/unklstewy/ads-bscope.git
cd ads-bscope

# Create environment configuration
cp .env.example .env
# Edit .env and set DB_PASSWORD and ADSB_API_KEY
```

### 2. Start Services

```bash
# Build and start all services
make build
make run

# Verify services are running
docker-compose ps
```

### 3. View Logs

```bash
# View all logs
make logs

# View specific service logs
docker-compose logs -f collector
docker-compose logs -f postgres
```

### 4. Stop Services

```bash
# Stop services (data persists)
make stop

# Stop and remove all data
make clean
```

## Configuration

### Environment Variables

Configuration is managed through environment variables defined in `.env`. See `.env.example` for all available options.

**Required:**
- `DB_PASSWORD`: PostgreSQL database password
- `ADSB_API_KEY`: API key for ADS-B data service

**Optional:**
- `ADSB_SOURCE`: Data source (default: `airplaneslive`)
- `LOG_LEVEL`: Logging level (`debug`, `info`, `warn`, `error`)
- `TZ`: Timezone (default: `UTC`)

### Configuration Files

Application configuration is stored in `configs/config.json`. This file can be edited while services are running (mount is read-only in container).

To apply configuration changes:
```bash
make restart
```

## Services

### PostgreSQL Database

**Container**: `adsbscope-db`
**Port**: `5432`
**Data**: Persisted in Docker volume `postgres_data`

Health check: `pg_isready -U adsbscope -d adsbscope`

**Resource Limits**:
- Memory: 512MB max, 256MB reserved

**Backup**:
```bash
# Create backup
docker exec adsbscope-db pg_dump -U adsbscope adsbscope > backup.sql

# Restore backup
docker exec -i adsbscope-db psql -U adsbscope adsbscope < backup.sql
```

### Collector Service

**Container**: `adsbscope-collector`
**Port**: `8080`
**Purpose**: Main ADS-B data collection and processing

Health check: `./collector --health-check`

**Resource Limits**:
- Memory: 256MB max, 128MB reserved

**Features**:
- Automatic database reconnection with exponential backoff
- Panic recovery with automatic restart
- Rate limiting for API calls
- Periodic cleanup of stale data

### Fetch Flightplans Service

**Container**: `adsbscope-fetch-flightplans`
**Purpose**: Periodic retrieval of flight plan data

**Resource Limits**:
- Memory: 128MB max, 64MB reserved

### Verification Tools

Two verification services are available via Docker profiles:

**verify-nasr**: NASR data verification
**verify-flightplans**: Flight plan data verification

These services run once and exit (not persistent).

To run verification tools:
```bash
# Run NASR verification
docker-compose --profile tools run verify-nasr

# Run flightplan verification
docker-compose --profile tools run verify-flightplans
```

## Development Workflow

### Local Development (without Docker)

For rapid iteration during development:

```bash
# Run locally (requires Go and PostgreSQL)
make dev-collector

# Or build and run manually
make build-collector
./bin/collector
```

### Docker Development

```bash
# Rebuild after code changes
make build

# Restart services
make restart

# View logs in real-time
make logs

# Run tests
make test

# Run tests with coverage
make test-coverage
```

### Building Individual Services

The Dockerfile supports building individual services using targets:

```bash
# Build specific service
docker build --target collector -t ads-bscope:collector .
docker build --target fetch-flightplans -t ads-bscope:fetch-flightplans .
docker build --target verify-nasr -t ads-bscope:verify-nasr .
docker build --target verify-flightplans -t ads-bscope:verify-flightplans .

# Run individual service
docker run --rm -it ads-bscope:collector
```

## Production Deployment

### Security Considerations

1. **Change default passwords**: Never use default `DB_PASSWORD`
2. **Secure API keys**: Store `ADSB_API_KEY` securely
3. **Network isolation**: Use Docker networks to isolate services
4. **Read-only mounts**: Configuration files are mounted read-only
5. **Non-root users**: All services run as UID 65534 (nobody)

### Production docker-compose.yml

For production, consider:

1. **External database**: Use managed PostgreSQL service
2. **Secrets management**: Use Docker secrets instead of environment variables
3. **Reverse proxy**: Add nginx/traefik for HTTPS
4. **Monitoring**: Add Prometheus/Grafana for metrics
5. **Log aggregation**: Use ELK stack or similar

Example production overrides:
```bash
# Create docker-compose.prod.yml with production settings
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### Resource Planning

Minimum recommended resources for production:

- **CPU**: 2 cores
- **RAM**: 2GB (database: 512MB, collector: 256MB, overhead: ~1GB)
- **Disk**: 10GB (grows with data retention)

### Scaling

To scale the collector service:
```bash
docker-compose up -d --scale collector=3
```

Note: Requires load balancer configuration for multiple instances.

## Troubleshooting

### Services won't start

```bash
# Check service status
docker-compose ps

# View logs for errors
docker-compose logs collector
docker-compose logs postgres

# Verify database is healthy
docker exec adsbscope-db pg_isready -U adsbscope
```

### Database connection errors

1. Verify database is running: `docker-compose ps postgres`
2. Check database logs: `docker-compose logs postgres`
3. Verify credentials in `.env`
4. Ensure database health check passes: `docker-compose ps`

### Out of memory errors

Check resource usage:
```bash
docker stats
```

Adjust memory limits in `docker-compose.yml`:
```yaml
deploy:
  resources:
    limits:
      memory: 512M  # Increase as needed
```

### Slow performance

1. **Database**: Tune PostgreSQL settings in `configs/postgresql.conf`
2. **Network**: Check for rate limiting in ADS-B API
3. **Disk**: Ensure adequate I/O performance for database volume

### Health check failures

```bash
# Check health status
docker inspect adsbscope-collector | grep -A 10 Health

# Manual health check
docker exec adsbscope-collector ./collector --health-check
```

### Data loss after restart

Data is persisted in Docker volumes. To verify:
```bash
# List volumes
docker volume ls | grep adsbscope

# Inspect volume
docker volume inspect ads-bscope_postgres_data
```

To backup data before cleanup:
```bash
# Backup volume
docker run --rm -v ads-bscope_postgres_data:/data -v $(pwd):/backup \
  alpine tar czf /backup/postgres-backup.tar.gz /data

# Restore volume
docker run --rm -v ads-bscope_postgres_data:/data -v $(pwd):/backup \
  alpine tar xzf /backup/postgres-backup.tar.gz -C /
```

## Advanced Topics

### Custom PostgreSQL Configuration

Create `configs/postgresql.conf` with custom settings:
```
# Example: Increase shared buffers for better performance
shared_buffers = 256MB
effective_cache_size = 1GB
```

Restart services to apply:
```bash
make restart
```

### Network Configuration

The default network uses subnet `172.28.0.0/16`. To customize:

Edit `docker-compose.yml`:
```yaml
networks:
  adsbscope-network:
    driver: bridge
    ipam:
      config:
        - subnet: 10.5.0.0/16
```

### Logs Directory

Logs are mounted to `./logs` for persistence. To change location:

Edit `docker-compose.yml`:
```yaml
volumes:
  - /path/to/logs:/app/logs
```

## Maintenance

### Regular Tasks

1. **Database vacuum**: Run weekly
   ```bash
   docker exec adsbscope-db vacuumdb -U adsbscope -d adsbscope -z
   ```

2. **Check disk usage**:
   ```bash
   docker system df
   ```

3. **Prune unused resources**:
   ```bash
   docker system prune -a --volumes
   ```

4. **Update images**:
   ```bash
   make build
   make restart
   ```

### Monitoring

View resource usage:
```bash
docker stats adsbscope-collector adsbscope-db
```

View service health:
```bash
docker-compose ps
```

## References

- [Docker Documentation](https://docs.docker.com/)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [PostgreSQL Docker Image](https://hub.docker.com/_/postgres)
- [Go Docker Best Practices](https://docs.docker.com/language/golang/)
