# ADS-B Scope PWA Interface

A Progressive Web Application for controlling telescopes to track aircraft using ADS-B data.

## Features

✨ **Modern UI**
- Dark theme optimized for astronomy
- Responsive design (mobile, tablet, desktop)
- Interactive sky map with Leaflet
- Real-time telemetry charts

🛩️ **Aircraft Tracking**
- Live aircraft positions on map
- Sortable/filterable aircraft list
- Click-to-select target aircraft
- Distance, altitude, and elevation data

🔭 **Telescope Control**
- Manual slew controls (8 directions + stop)
- Automatic aircraft tracking
- Alt/Az and RA/Dec position display
- Altitude limits warning (20-80° for Seestar)
- Real-time altitude chart

👥 **Multi-User Ready**
- Role-based access control (Admin, Observer, Viewer, Guest)
- Session-based authentication
- User management (when backend is ready)

📱 **PWA Features**
- Installable on mobile devices
- Offline support with service worker
- Fast loading with caching
- Native app-like experience

## Quick Start

### Local Development

1. **Start the development server:**
   ```bash
   cd web
   python3 serve.py
   ```

2. **Open in browser:**
   - Navigate to http://localhost:8000
   - Use demo credentials: `admin` / `admin`

3. **Try the interface:**
   - Click on aircraft in the map or list to select
   - Use telescope controls to simulate slewing
   - Watch real-time telemetry updates

### Testing PWA Features

To test PWA installation and offline mode:

1. Open in **Chrome/Edge** (best PWA support)
2. Look for the **install icon** in the address bar
3. Install the app
4. Test offline mode:
   - Open DevTools → Application → Service Workers
   - Check "Offline" mode
   - Refresh - the app should still load

## Project Structure

```
web/
├── static/
│   ├── index.html          # Main app shell
│   ├── manifest.json       # PWA manifest
│   ├── sw.js              # Service worker
│   ├── css/
│   │   └── main.css       # All styles
│   ├── js/
│   │   ├── app.js         # Main application logic
│   │   ├── api.js         # Mock API client
│   │   ├── components/    # Future web components
│   │   └── utils/         # Utility functions
│   └── icons/
│       └── favicon.svg    # App icon
├── serve.py               # Development server
└── README.md             # This file
```

## Current Status

### ✅ Completed (Prototype Phase)

- [x] HTML app shell with responsive layout
- [x] Dark theme CSS with animations
- [x] Mock API client with fake data
- [x] Leaflet map integration
- [x] Aircraft list with search/sort
- [x] Telescope control UI
- [x] Telemetry dashboard with Chart.js
- [x] Toast notifications
- [x] PWA manifest and service worker
- [x] Authentication UI

### 🚧 Next Steps (Backend Integration)

- [ ] Go HTTP server with chi router
- [ ] Real API endpoints (`/api/v1/...`)
- [ ] WebSocket server for real-time updates
- [ ] Database-backed authentication
- [ ] RBAC middleware
- [ ] Connect to actual ADS-B data source
- [ ] Connect to ASCOM Alpaca telescope
- [ ] User management interface
- [ ] Session management
- [ ] Audit logging

### 🎯 Future Enhancements

- [ ] WebGL sky chart (more accurate celestial rendering)
- [ ] Constellation overlays
- [ ] Aircraft trajectory prediction paths
- [ ] Multiple telescope support
- [ ] Image gallery (captured photos)
- [ ] Session recording/playback
- [ ] Mobile native apps (iOS/Android)
- [ ] Push notifications for tracking events

## Mock Data

The current prototype uses mock data defined in `js/api.js`:

**Mock Aircraft:**
- 4 aircraft with varying positions, altitudes, speeds
- Simulated movement (slight position changes)
- San Francisco Bay Area locations (example)

**Mock Telescope:**
- Initial position: Alt 45.5°, Az 180°
- Simulated tracking movements
- Altitude limits: 20-80° (Seestar constraints)

**Demo Users:**
- `admin` / `admin` → Admin role
- Any username/password → Observer role

## Technology Stack

### Frontend
- **Vanilla JavaScript** (ES6 modules)
- **Leaflet.js** 1.9.4 - Interactive maps
- **Chart.js** 4.4.1 - Telemetry charts
- **CSS Grid/Flexbox** - Responsive layout
- **Service Worker** - Offline support

### Backend (Planned)
- **Go** with chi router
- **PostgreSQL** database
- **WebSocket** (gorilla/websocket)
- **JWT** authentication (golang-jwt/jwt)
- **Docker** deployment

## API Design (Future)

When the Go backend is implemented, the API will follow this structure:

```
POST   /api/v1/auth/login
POST   /api/v1/auth/logout
GET    /api/v1/auth/refresh

GET    /api/v1/users
POST   /api/v1/users
GET    /api/v1/users/:id
PUT    /api/v1/users/:id
DELETE /api/v1/users/:id

GET    /api/v1/aircraft
GET    /api/v1/aircraft/:icao

GET    /api/v1/telescope/status
POST   /api/v1/telescope/slew
POST   /api/v1/telescope/track/:icao
POST   /api/v1/telescope/stop
POST   /api/v1/telescope/abort

GET    /api/v1/system/status
GET    /api/v1/system/health

WS     /api/v1/ws              # WebSocket for real-time updates
```

## Browser Compatibility

**Recommended:**
- Chrome 90+
- Edge 90+
- Safari 14+
- Firefox 88+

**PWA Features:**
- Chrome/Edge: Full support
- Safari (iOS): Installable, limited service worker
- Firefox: Partial support

## Development Notes

### Service Worker Caching

The service worker caches:
- Static assets (HTML, CSS, JS)
- External libraries (Leaflet, Chart.js)
- App manifest

Cache strategy: **Cache-first with background update**
- Serves from cache immediately
- Updates cache in background from network
- Falls back to cache if offline

### Mock API Behavior

The mock API simulates:
- Network latency (100-500ms delays)
- Authentication state (sessionStorage)
- Data changes (aircraft movement)
- Error scenarios (altitude limits)

### Responsive Breakpoints

- Desktop: 1024px+
- Tablet: 768px - 1024px
- Mobile: < 768px

## Troubleshooting

**Service worker not registering:**
- Must be served over HTTPS or localhost
- Check browser console for errors
- Try incognito mode to clear cache

**Map not loading:**
- Check internet connection (Leaflet tiles from CDN)
- Check browser console for Leaflet errors
- Ensure port 8000 is not blocked

**Aircraft not appearing:**
- Mock data uses San Francisco coordinates
- Zoom/pan the map to see markers
- Check console for JavaScript errors

**Authentication not persisting:**
- Uses sessionStorage (clears on tab close)
- Use localStorage for longer persistence (future)

## Contributing

This is the prototype phase. Once the Go backend is ready:

1. Replace `api.js` mock implementations with real API calls
2. Add WebSocket client for real-time updates
3. Implement proper error handling
4. Add loading states and skeleton screens
5. Enhance mobile UX
6. Add E2E tests (Playwright/Cypress)

## License

TBD (MIT or Apache 2.0)

## Related Documentation

- Main project README: `../README.md`
- Roadmap: `../ROADMAP.md`
- Docker setup: `../DOCKER.md`
- WARP guidelines: `../WARP.md`

---

**Last Updated:** January 2026  
**Status:** Prototype (Frontend only)  
**Next Milestone:** Backend API integration
