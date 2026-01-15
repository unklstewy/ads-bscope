# Running the Web Server with Docker

The web server has been integrated into the docker-compose setup.

## Architecture

- **Port 8080**: Web Server (PWA + REST API)
- **Port 5432**: PostgreSQL Database
- **Collector**: Runs without exposed port (feeds database)

## Quick Start

### 1. Stop any existing services
```bash
docker-compose down
```

### 2. Build and start all services
```bash
docker-compose up -d --build
```

This will start:
- PostgreSQL database
- Web server (on port 8080)
- ADS-B collector (background)

### 3. Access the PWA
Open http://localhost:8080 in your browser and login with:
- **Username**: `admin`
- **Password**: `admin`

## Individual Service Management

### Start only the web server and database
```bash
docker-compose up -d postgres web-server
```

### View web server logs
```bash
docker-compose logs -f web-server
```

### View collector logs
```bash
docker-compose logs -f collector
```

### Restart web server
```bash
docker-compose restart web-server
```

## Configuration

The web server uses these environment variables (set in docker-compose.yml):
- `ADS_BSCOPE_DB_HOST=postgres` - Database hostname
- `ADS_BSCOPE_DB_PORT=5432` - Database port
- `JWT_SECRET` - Secret for JWT tokens (default: dev-secret-change-in-production)
- `CONFIG_PATH=/app/configs/config.json` - Config file path

## Development

For development, you can run the web server locally instead:

```bash
# Make sure PostgreSQL is running
docker-compose up -d postgres

# Run web server locally
./bin/web-server --port 8080
```

This allows faster iteration without rebuilding the Docker image.

## Troubleshooting

### Web server won't start
Check logs:
```bash
docker-compose logs web-server
```

### Port 8080 already in use
Find what's using it:
```bash
lsof -i :8080
```

Stop the old collector container:
```bash
docker stop adsbscope-app
docker rm adsbscope-app
```

### Database connection issues
Verify database is running:
```bash
docker-compose ps postgres
docker exec -i adsbscope-db psql -U adsbscope -d adsbscope -c "SELECT version();"
```

### Web server not serving static files
Verify the web/static directory is mounted:
```bash
docker-compose exec web-server ls -la /app/web/static
```

## Health Checks

The web server has a health check endpoint:
```bash
curl http://localhost:8080/api/v1/system/status
```

Should return:
```json
{
  "telescope": false,
  "adsb": false,
  "tracking": false,
  "message": "System status endpoint - to be implemented"
}
```

## Production Deployment

For production:

1. Set secure JWT secret:
   ```bash
   export JWT_SECRET=$(openssl rand -base64 32)
   ```

2. Change admin password immediately after first login

3. Set up HTTPS (use nginx or Caddy as reverse proxy)

4. Update database password:
   ```bash
   export DB_PASSWORD=$(openssl rand -base64 32)
   ```

5. Use proper resource limits in docker-compose.yml

## Next Steps

- [ ] Wire up aircraft endpoints to database
- [ ] Wire up telescope endpoints to Alpaca
- [ ] Implement WebSocket for real-time updates
- [ ] Add HTTPS support
- [ ] Configure proper logging
