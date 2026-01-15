#!/bin/bash
# Database Setup and Migration Script for ADS-B Scope

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}🗄️  ADS-B Scope Database Setup${NC}"
echo ""

# Configuration
DB_CONTAINER="adsbscope-db"
DB_USER="adsbscope"
DB_NAME="adsbscope"
DB_PASSWORD="${DB_PASSWORD:-changeme}"

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker is not running. Please start Docker and try again.${NC}"
    exit 1
fi

# Check if database container exists
if ! docker ps -a --format '{{.Names}}' | grep -q "^${DB_CONTAINER}$"; then
    echo -e "${BLUE}📦 Creating PostgreSQL container...${NC}"
    docker run -d \
        --name ${DB_CONTAINER} \
        -e POSTGRES_DB=${DB_NAME} \
        -e POSTGRES_USER=${DB_USER} \
        -e POSTGRES_PASSWORD=${DB_PASSWORD} \
        -p 5432:5432 \
        postgres:16-alpine
    
    echo -e "${GREEN}✓ PostgreSQL container created${NC}"
    echo "  Waiting for database to start..."
    sleep 5
else
    # Start container if it's not running
    if ! docker ps --format '{{.Names}}' | grep -q "^${DB_CONTAINER}$"; then
        echo -e "${BLUE}▶️  Starting existing PostgreSQL container...${NC}"
        docker start ${DB_CONTAINER}
        sleep 3
    fi
    echo -e "${GREEN}✓ PostgreSQL container is running${NC}"
fi

# Wait for database to be ready
echo -e "${BLUE}⏳ Waiting for database to be ready...${NC}"
for i in {1..30}; do
    if docker exec ${DB_CONTAINER} pg_isready -U ${DB_USER} > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Database is ready${NC}"
        break
    fi
    if [ $i -eq 30 ]; then
        echo -e "${RED}❌ Database did not become ready in time${NC}"
        exit 1
    fi
    sleep 1
done

# Apply main schema if it exists
if [ -f "internal/db/schema.sql" ]; then
    echo -e "${BLUE}📝 Applying main schema...${NC}"
    docker exec -i ${DB_CONTAINER} psql -U ${DB_USER} -d ${DB_NAME} < internal/db/schema.sql > /dev/null 2>&1 || true
    echo -e "${GREEN}✓ Main schema applied${NC}"
fi

# Apply migrations
echo -e "${BLUE}📝 Applying migrations...${NC}"
MIGRATION_DIR="internal/db/migrations"

if [ -d "$MIGRATION_DIR" ]; then
    for migration in $(ls -1 $MIGRATION_DIR/*.sql 2>/dev/null | sort); do
        filename=$(basename "$migration")
        echo "  Applying ${filename}..."
        docker exec -i ${DB_CONTAINER} psql -U ${DB_USER} -d ${DB_NAME} < "$migration"
    done
    echo -e "${GREEN}✓ All migrations applied${NC}"
else
    echo -e "${RED}⚠️  No migrations directory found${NC}"
fi

# Verify tables
echo ""
echo -e "${BLUE}📊 Database Tables:${NC}"
docker exec -i ${DB_CONTAINER} psql -U ${DB_USER} -d ${DB_NAME} -c "\dt" | grep -E "public \|" || echo "No tables found"

# Check users
echo ""
echo -e "${BLUE}👥 Users:${NC}"
docker exec -i ${DB_CONTAINER} psql -U ${DB_USER} -d ${DB_NAME} -c "SELECT id, username, role, is_active, created_at FROM users;" 2>/dev/null || echo "Users table not yet created"

# Print connection info
echo ""
echo -e "${GREEN}✅ Database setup complete!${NC}"
echo ""
echo -e "${BLUE}Connection details:${NC}"
echo "  Host: localhost"
echo "  Port: 5432"
echo "  Database: ${DB_NAME}"
echo "  User: ${DB_USER}"
echo "  Password: ${DB_PASSWORD}"
echo ""
echo -e "${BLUE}Default admin user:${NC}"
echo "  Username: admin"
echo "  Password: admin"
echo "  ${RED}⚠️  Change this password immediately in production!${NC}"
echo ""
echo -e "${BLUE}Connection string:${NC}"
echo "  postgresql://${DB_USER}:${DB_PASSWORD}@localhost:5432/${DB_NAME}?sslmode=disable"
