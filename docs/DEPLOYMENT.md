# Deployment Guide

This guide covers deploying ADS-B Scope across different environments and skill levels.

## Table of Contents
- [Quick Start (Standalone)](#quick-start-standalone)
- [Docker Deployment](#docker-deployment)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Configuration Reference](#configuration-reference)

---

## Quick Start (Standalone)

**Use Case**: Running on a single machine (Raspberry Pi, laptop, desktop) with all services on localhost.

**Skill Level**: Beginner to Intermediate

**Requirements**:
- Go 1.21+ (install via Homebrew: `brew install go`)
- PostgreSQL 16+ running locally
- Optional: Dynamic DNS (e.g., Duck DNS, No-IP) for remote access
- Optional: Nginx for HTTPS reverse proxy

### 1. Setup PostgreSQL

```bash
# macOS (Homebrew)
brew install postgresql@16
brew services start postgresql@16

# Linux (Debian/Ubuntu)
sudo apt install postgresql-16
sudo systemctl start postgresql

# Create database and user
psql -U postgres
CREATE DATABASE adsbscope;
CREATE USER adsbscope WITH PASSWORD 'changeme';
GRANT ALL PRIVILEGES ON DATABASE adsbscope TO adsbscope;
\q
```

### 2. Configure Application

The default `configs/config.json` is pre-configured for localhost deployment:

```json
{
  "database": {
    "host": "localhost",
    "port": 5432,
    "database": "adsbscope",
    "username": "adsbscope",
    "password": "changeme"
  }
}
```

**For production**: Use environment variables for sensitive data:

```bash
export ADS_BSCOPE_DB_PASSWORD="your-secure-password"
```

### 3. Build and Run

```bash
# Build the application
go build -o bin/ads-bscope ./cmd/ads-bscope
go build -o bin/collector ./cmd/collector

# Run the collector
./bin/collector

# Run the web application (in another terminal)
./bin/ads-bscope
```

Access the application at: `http://localhost:8080`

### 4. Optional: Setup Nginx Reverse Proxy

For remote access with HTTPS:

```nginx
# /etc/nginx/sites-available/adsbscope
server {
    listen 80;
    server_name your-dynamic-dns-hostname.duckdns.org;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket support (for real-time updates)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

Enable and configure SSL with Let's Encrypt:

```bash
sudo ln -s /etc/nginx/sites-available/adsbscope /etc/nginx/sites-enabled/
sudo certbot --nginx -d your-dynamic-dns-hostname.duckdns.org
sudo nginx -t && sudo systemctl reload nginx
```

### 5. Optional: Systemd Service

Create `/etc/systemd/system/adsbscope-collector.service`:

```ini
[Unit]
Description=ADS-B Scope Collector Service
After=network.target postgresql.service

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi/ads-bscope
Environment="ADS_BSCOPE_DB_PASSWORD=your-secure-password"
ExecStart=/home/pi/ads-bscope/bin/collector
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable adsbscope-collector
sudo systemctl start adsbscope-collector
```

---

## Docker Deployment

**Use Case**: Simplified deployment with Docker Compose on a single host.

**Skill Level**: Intermediate

**Requirements**:
- Docker 24+ and Docker Compose V2
- Basic understanding of containers
- Optional: Traefik or Nginx for reverse proxy

### 1. Deploy with Docker Compose

The included `docker-compose.yml` automatically configures networking:

```bash
# Start services
docker compose up -d

# View logs
docker compose logs -f

# Stop services
docker compose down
```

The application will be available at: `http://localhost:8080`

### 2. Environment Configuration

Create a `.env` file for secrets:

```bash
# .env
DB_PASSWORD=your-secure-password
```

The Docker Compose file automatically sets `ADS_BSCOPE_DB_HOST=adsbscope-db` to use the internal Docker network.

### 3. Custom Hostnames and Networking

For advanced networking (e.g., Traefik, custom networks):

```yaml
# docker-compose.override.yml
services:
  app:
    environment:
      ADS_BSCOPE_DB_HOST: my-postgres-service.internal
    networks:
      - my-custom-network

networks:
  my-custom-network:
    external: true
```

### 4. Reverse Proxy with Traefik

Add Traefik labels for automatic HTTPS:

```yaml
services:
  app:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.adsbscope.rule=Host(`adsbscope.example.com`)"
      - "traefik.http.routers.adsbscope.entrypoints=websecure"
      - "traefik.http.routers.adsbscope.tls.certresolver=letsencrypt"
```

---

## Kubernetes Deployment

**Use Case**: Multi-region observatories, high availability, or cluster-based deployments.

**Skill Level**: Advanced

**Requirements**:
- Kubernetes 1.27+ cluster
- kubectl configured
- Basic understanding of K8s resources (Deployments, Services, ConfigMaps, Secrets)
- Optional: Helm for package management

### 1. Namespace and ConfigMap

```yaml
# k8s/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: adsbscope
```

```yaml
# k8s/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: adsbscope-config
  namespace: adsbscope
data:
  config.json: |
    {
      "server": {
        "port": "8080",
        "host": "0.0.0.0"
      },
      "database": {
        "driver": "postgres",
        "host": "postgres-service",
        "port": 5432,
        "database": "adsbscope",
        "username": "adsbscope"
      },
      "observer": {
        "name": "CLT Primary Observatory",
        "latitude": 35.1871,
        "longitude": -80.9218,
        "elevation": 230.0,
        "timezone": "America/New_York"
      }
    }
```

### 2. Secrets

```yaml
# k8s/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: adsbscope-secrets
  namespace: adsbscope
type: Opaque
stringData:
  db-password: your-secure-password
  # Add other sensitive data here
```

### 3. PostgreSQL Deployment

```yaml
# k8s/postgres.yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres-service
  namespace: adsbscope
spec:
  ports:
    - port: 5432
  selector:
    app: postgres
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: adsbscope
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:16-alpine
        env:
        - name: POSTGRES_DB
          value: adsbscope
        - name: POSTGRES_USER
          value: adsbscope
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: adsbscope-secrets
              key: db-password
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
      volumes:
      - name: postgres-storage
        persistentVolumeClaim:
          claimName: postgres-pvc
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: adsbscope
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

### 4. Application Deployment

```yaml
# k8s/app.yaml
apiVersion: v1
kind: Service
metadata:
  name: adsbscope-service
  namespace: adsbscope
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: adsbscope
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: adsbscope
  namespace: adsbscope
spec:
  replicas: 1
  selector:
    matchLabels:
      app: adsbscope
  template:
    metadata:
      labels:
        app: adsbscope
    spec:
      containers:
      - name: adsbscope
        image: adsbscope:latest
        env:
        - name: ADS_BSCOPE_DB_HOST
          value: postgres-service
        - name: ADS_BSCOPE_DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: adsbscope-secrets
              key: db-password
        - name: CONFIG_PATH
          value: /app/configs/config.json
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /app/configs
      volumes:
      - name: config
        configMap:
          name: adsbscope-config
```

### 5. Ingress with Cert-Manager

```yaml
# k8s/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: adsbscope-ingress
  namespace: adsbscope
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    kubernetes.io/ingress.class: nginx
spec:
  tls:
  - hosts:
    - adsbscope.example.com
    secretName: adsbscope-tls
  rules:
  - host: adsbscope.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: adsbscope-service
            port:
              number: 8080
```

### 6. Deploy to Kubernetes

```bash
# Apply all manifests
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/postgres.yaml
kubectl apply -f k8s/app.yaml
kubectl apply -f k8s/ingress.yaml

# Check status
kubectl get pods -n adsbscope
kubectl logs -f -n adsbscope deployment/adsbscope

# Access application
# Via Ingress: https://adsbscope.example.com
# Via port-forward: kubectl port-forward -n adsbscope svc/adsbscope-service 8080:8080
```

### 7. Multi-Region Deployment

For multiple observatories, create separate namespaces per location:

```bash
# Deploy Charlotte observatory
kubectl apply -f k8s/namespaces/charlotte/

# Deploy Atlanta observatory
kubectl apply -f k8s/namespaces/atlanta/

# Each namespace has its own:
# - ConfigMap with observer location
# - PostgreSQL instance
# - Application deployment
```

Or use a shared PostgreSQL with region-specific application deployments:

```yaml
# k8s/app-charlotte.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: adsbscope-charlotte
  namespace: adsbscope
spec:
  template:
    spec:
      containers:
      - name: adsbscope
        env:
        - name: ADS_BSCOPE_DB_HOST
          value: postgres-service  # Shared database
        volumeMounts:
        - name: config
          mountPath: /app/configs
      volumes:
      - name: config
        configMap:
          name: adsbscope-config-charlotte  # Region-specific config
```

---

## Configuration Reference

### Environment Variables

All configuration can be overridden via environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `ADS_BSCOPE_PORT` | HTTP server port | `8080` |
| `ADS_BSCOPE_DB_HOST` | Database hostname | `localhost`, `postgres-service` |
| `ADS_BSCOPE_DB_PASSWORD` | Database password | `secure-password` |
| `ADS_BSCOPE_TELESCOPE_URL` | Alpaca server URL | `http://192.168.1.100:11111` |
| `ADS_BSCOPE_ADSB_API_KEY` | ADS-B API key | `your-api-key` |
| `ADS_BSCOPE_FLIGHTAWARE_API_KEY` | FlightAware API key | `your-fa-key` |
| `CONFIG_PATH` | Config file path | `/app/configs/config.json` |

### Configuration Precedence

1. **Environment variables** (highest priority)
2. **Config file** (`configs/config.json`)
3. **Default values** (lowest priority)

This allows flexible deployment:
- **Development**: Use default `config.json` with `localhost`
- **Docker**: Override with environment variables
- **Kubernetes**: Use ConfigMaps + Secrets

### Port Mapping

Default ports:
- **8080**: Web application (HTTP)
- **5432**: PostgreSQL database
- **11111**: ASCOM Alpaca (telescope)

Customize in `config.json` or via environment variables.

---

## Future: Setup Wizard

A setup wizard is planned to automate deployment configuration:

```bash
# Interactive setup
./bin/ads-bscope setup

# Questions:
# 1. Deployment type? [standalone, docker, kubernetes]
# 2. Database host? [localhost, custom]
# 3. Observer location? [latitude, longitude, elevation]
# 4. Enable HTTPS? [yes, no]
# 5. Generate deployment files? [yes, no]

# Output:
# - configs/config.json
# - docker-compose.yml (if Docker)
# - k8s/*.yaml (if Kubernetes)
# - nginx.conf (if HTTPS)
```

This will provide a guided experience for all skill levels.
