# ADS-B Scope PWA Status

## ✅ Completed

### UI/Frontend (Complete)
- ✅ Responsive PWA interface with dark astronomy theme
- ✅ Login/authentication UI  
- ✅ Interactive Leaflet sky map
- ✅ Aircraft list with search/sort
- ✅ Telescope control panel with manual slew grid
- ✅ Real-time telemetry dashboard with Chart.js
- ✅ Toast notifications
- ✅ PWA manifest and service worker
- ✅ Mock API for UI testing
- ✅ Improved sizing/scaling for high-DPI displays

### Backend Infrastructure (Partially Complete)
- ✅ Go web server with chi router
- ✅ Static file serving
- ✅ JWT authentication system
- ✅ Password hashing with bcrypt
- ✅ User database schema and repository
- ✅ CORS middleware
- ✅ Auth endpoints (`/api/v1/auth/login`, `/logout`)
- ✅ Database connection setup

## 🚧 In Progress / To-Do

### Database Setup
- [ ] Run migration script (001_create_auth_tables.sql)
- [ ] Create proper admin user with hashed password
- [ ] Test user authentication flow

### API Endpoints (Placeholders Exist, Need Implementation)
- [ ] `/api/v1/aircraft` - Connect to pkg/adsb
- [ ] `/api/v1/aircraft/{icao}` - Get specific aircraft
- [ ] `/api/v1/telescope/status` - Connect to pkg/alpaca
- [ ] `/api/v1/telescope/slew` - Telescope slewing
- [ ] `/api/v1/telescope/track/{icao}` - Start tracking
- [ ] `/api/v1/telescope/stop` - Stop tracking
- [ ] `/api/v1/telescope/abort` - Emergency stop
- [ ] `/api/v1/system/status` - System health check

### WebSocket (Not Started)
- [ ] WebSocket hub implementation
- [ ] Real-time aircraft position updates
- [ ] Real-time telescope status updates
- [ ] Client connection management

### Frontend Integration (Not Started)
- [ ] Update `web/static/js/api.js` to use real endpoints
- [ ] Add Authorization header to API calls
- [ ] Handle JWT token storage and refresh
- [ ] Connect WebSocket for live updates
- [ ] Error handling and loading states

## 🚀 Quick Start

### Running the PWA (Mock Data)
```bash
# Uses mock API, no database required
./start-pwa.sh

# Or manually:
cd web
python3 serve.py
```

Open http://localhost:8000 and login with `admin` / `admin`

### Running the Go Backend (Real Integration - Coming Soon)
```bash
# 1. Ensure PostgreSQL is running
docker-compose up -d postgres

# 2. Run migrations
psql -h localhost -U ads_bscope -d ads_bscope < internal/db/migrations/001_create_auth_tables.sql

# 3. Build and run server
go build -o bin/web-server ./cmd/web-server
./bin/web-server --config configs/config.json --port 8080
```

Open http://localhost:8080 and login with database credentials

## 📂 Project Structure

```
ads-bscope/
├── web/
│   ├── static/                    # PWA frontend
│   │   ├── index.html            # Main app shell
│   │   ├── manifest.json         # PWA manifest
│   │   ├── sw.js                 # Service worker
│   │   ├── css/main.css          # Styles (optimized sizing)
│   │   └── js/
│   │       ├── app.js            # Main app logic
│   │       └── api.js            # API client (currently mock)
│   ├── serve.py                  # Development server (mock API)
│   └── README.md                 # PWA documentation
│
├── cmd/
│   └── web-server/
│       └── main.go               # Go HTTP server ✅
│
├── internal/
│   ├── auth/
│   │   └── auth.go               # JWT & password hashing ✅
│   ├── db/
│   │   ├── user_repository.go    # User database operations ✅
│   │   └── migrations/
│   │       └── 001_create_auth_tables.sql  # Database schema ✅
│   └── api/                      # (To be created)
│       ├── handlers/             # API route handlers
│       └── websocket/            # WebSocket hub
│
├── pkg/
│   ├── adsb/                     # ADS-B data (existing)
│   ├── alpaca/                   # Telescope control (existing)
│   ├── coordinates/              # Coordinate transforms (existing)
│   └── tracking/                 # Tracking algorithms (existing)
│
├── start-pwa.sh                  # Quick start script ✅
└── PWA_STATUS.md                 # This file
```

## 🔧 Next Implementation Steps

### 1. Database Migration
Create helper script `scripts/setup-db.sh`:
```bash
#!/bin/bash
# Load migrations
psql $DATABASE_URL < internal/db/schema.sql
psql $DATABASE_URL < internal/db/migrations/001_create_auth_tables.sql

# Create admin user with proper hash
HASH=$(htpasswd -bnBC 10 "" admin | tr -d ':\n')
psql $DATABASE_URL -c "
  INSERT INTO users (username, email, password_hash, role)
  VALUES ('admin', 'admin@ads-bscope.local', '$HASH', 'admin')
  ON CONFLICT (username) DO NOTHING;
"
```

### 2. Wire Up Aircraft Endpoints
In `cmd/web-server/main.go`:
```go
func (s *Server) handleGetAircraft(w http.ResponseWriter, r *http.Request) {
    // Use existing pkg/adsb to get aircraft from database
    repo := db.NewAircraftRepository(s.db)
    aircraft, err := repo.GetVisible(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    respondJSON(w, http.StatusOK, map[string]interface{}{
        "aircraft": aircraft,
    })
}
```

### 3. Wire Up Telescope Endpoints
```go
func (s *Server) handleGetTelescopeStatus(w http.ResponseWriter, r *http.Request) {
    // Use existing pkg/alpaca client
    client := alpaca.NewClient(s.cfg.Telescope.BaseURL, s.cfg.Telescope.DeviceNumber)
    status, err := client.GetStatus(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    respondJSON(w, http.StatusOK, status)
}
```

### 4. Implement WebSocket Hub
Create `internal/api/websocket/hub.go`:
```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan []byte
    register   chan *Client
    unregister chan *Client
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.clients[client] = true
        case client := <-h.unregister:
            delete(h.clients, client)
        case message := <-h.broadcast:
            for client := range h.clients {
                client.send <- message
            }
        }
    }
}
```

### 5. Update Frontend API Client
In `web/static/js/api.js`, replace mock functions:
```javascript
export const aircraft = {
    async getAll() {
        const response = await fetch('/api/v1/aircraft', {
            headers: {
                'Authorization': `Bearer ${getToken()}`
            }
        });
        return response.json();
    }
};
```

## 📊 Progress Tracking

**Frontend**: 100% complete (with mock data)
**Backend Infrastructure**: 60% complete
**API Implementation**: 10% complete (auth only)
**WebSocket**: 0% complete
**Integration**: 0% complete

**Estimated Time to Full Integration**: 4-6 hours of focused development

## 🎯 MVP Definition

For a functioning MVP, we need:
1. ✅ PWA interface
2. ✅ Go HTTP server
3. ✅ Authentication
4. ⏳ Aircraft data endpoints
5. ⏳ Telescope control endpoints  
6. ⏳ WebSocket updates
7. ⏳ Frontend integration

**Current Status**: ~50% to MVP

## 🔒 Security Notes

- JWT secret should be set via environment variable in production
- Default admin password must be changed immediately
- HTTPS required for production deployment
- CORS currently allows all origins (development only)
- Rate limiting not yet implemented

## 📝 Documentation

- PWA UI: See `web/README.md`
- API Design: See plan document
- Architecture: See `WARP.md` and `ROADMAP.md`
- Database Schema: See `internal/db/schema.sql`

---

**Status as of**: January 14, 2026  
**Last Updated**: UI scaling improvements, Go backend framework complete  
**Next Milestone**: Wire up aircraft and telescope endpoints
