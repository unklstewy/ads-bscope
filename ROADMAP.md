# ADS-B Scope Roadmap

This document outlines the future development plans for ads-bscope, including major features, enhancements, and architectural improvements.

## Vision

Transform ads-bscope from a command-line telescope control system into a comprehensive, multi-user Progressive Web Application with rich terminal interfaces and federated authentication, making aircraft tracking and telescope control accessible to astronomy enthusiasts worldwide.

---

## Current Status (v0.1 - MVP)

**Completed Features:**
- ✅ Core ADS-B data acquisition (Airplanes.live API)
- ✅ ASCOM Alpaca telescope interface
- ✅ PostgreSQL database with aircraft tracking
- ✅ Coordinate transformations (Alt/Az and Equatorial)
- ✅ Meridian flip detection and tracking limits
- ✅ Flight plan integration
- ✅ Docker containerization
- ✅ Comprehensive test coverage (>70% core packages)

**Current Limitations:**
- Command-line only interface
- Single-user operation
- No web interface
- Manual configuration required
- No authentication system

---

## Phase 1: Enhanced Terminal Interface (Q1 2026)

### TermGL Terminal Graphics

**Objective:** Create a rich, visual terminal interface with advanced geometric rendering for real-time sky visualization and telescope control.

**Why TermGL over bubble-tea:**
- Advanced geometric shape control
- Real-time sky chart rendering in terminal
- Better visual feedback for telescope tracking
- Superior drawing primitives for aircraft trails
- Still accessible via SSH and limited terminals

**Features:**
- 🎯 Real-time sky map with aircraft positions
- 🎯 Telescope field of view visualization
- 🎯 Aircraft trajectory prediction paths
- 🎯 Interactive telescope control (slew, track, stop)
- 🎯 Altitude/azimuth grid overlay
- 🎯 Constellation outlines
- 🎯 Status dashboard with telemetry
- 🎯 Multi-panel layout (map, logs, stats, controls)

**Technical Considerations:**
- Explore TermGL library capabilities
- Fallback to bubble-tea for minimal terminals
- Performance optimization for 60fps updates
- Color themes for different terminal capabilities

**Definition of Done:**
- TermGL-based TUI application running
- Real-time aircraft tracking display
- Interactive telescope control functional
- Documentation for TUI usage
- Performance benchmark: <16ms frame time

---

## Phase 2: Progressive Web Application (Q2 2026)

### Multi-User PWA with Role-Based Access Control

**Objective:** Build a Progressive Web Application that brings telescope control to any device with a web browser, supporting multiple users with different permission levels.

**Core Features:**

#### 2.1 Web Interface
- 📱 Responsive PWA design (mobile, tablet, desktop)
- 🗺️ Interactive sky map (WebGL/Canvas)
- 📊 Real-time telemetry dashboard
- 🎮 Touch-friendly telescope controls
- 📸 Live telescope camera feed integration
- 🔔 Push notifications for tracking events
- 💾 Offline capability with service workers
- 📥 PWA installation (Add to Home Screen)

#### 2.2 Multi-User Architecture
- 👥 User registration and profile management
- 🔐 Session management with JWT tokens
- 🏢 Organization/team support
- 👨‍👩‍👧‍👦 Shared telescope access scheduling
- 📜 Audit logging for all telescope operations
- ⚙️ Per-user configuration and preferences

#### 2.3 Role-Based Access Control (RBAC)

**User Roles:**

**Administrator**
- Full system configuration
- User management
- Telescope hardware settings
- Database maintenance
- Security policy configuration

**Observer**
- Telescope control (slew, track, stop)
- Aircraft target selection
- Configuration overrides (tracking limits, regions)
- Session scheduling
- View all telemetry

**Viewer**
- Read-only access
- View current telescope status
- View aircraft positions
- View tracking history
- No control permissions

**Guest**
- Public dashboard view
- Limited telemetry (no sensitive data)
- Read-only sky map
- No telescope control

**Features:**
- Granular permissions per resource
- Role inheritance
- Custom role creation
- Time-based access (scheduled sessions)
- IP-based restrictions

#### 2.4 Technology Stack
- **Frontend:** React or Vue.js + TypeScript
- **State Management:** Redux or Vuex
- **UI Components:** Material-UI or Vuetify
- **Maps:** Leaflet or Mapbox GL
- **Charts:** D3.js or Chart.js
- **WebSocket:** Real-time updates
- **Service Worker:** Offline support
- **Backend API:** RESTful + WebSocket endpoints

**Definition of Done:**
- PWA installable on mobile devices
- Multi-user authentication working
- All 4 user roles implemented
- Role-based telescope access enforced
- Responsive UI on mobile, tablet, desktop
- Offline mode functional
- Security audit passed
- Performance: <2s initial load, <100ms API response

---

## Phase 3: Federated Authentication (Q2-Q3 2026)

### OAuth 2.0 / Auth0 Integration

**Objective:** Eliminate password management burden by integrating with popular identity providers, enabling users to sign in with existing accounts.

**Supported Identity Providers:**
- 🔵 Google (OAuth 2.0)
- 🔵 Facebook (OAuth 2.0)
- 🔵 GitHub (OAuth 2.0)
- 🔵 Microsoft (Azure AD)
- 🔵 Apple (Sign in with Apple)
- 🔵 Twitter/X (OAuth 2.0)

**Implementation Options:**

#### Option A: Auth0 (Recommended)
**Pros:**
- Turnkey solution with all providers
- Built-in user management UI
- Advanced security features (MFA, anomaly detection)
- Compliance ready (SOC 2, GDPR, HIPAA)
- Extensive documentation and SDKs

**Cons:**
- Cost (free tier: 7,500 MAU, then paid)
- External dependency

#### Option B: Self-Hosted OAuth
**Pros:**
- Full control over authentication flow
- No external costs
- No data sharing with third parties

**Cons:**
- Significant development effort
- Security responsibility
- Maintenance burden

**Features:**
- 🔐 Social login buttons on login page
- 🔗 Account linking (merge local + OAuth accounts)
- 👤 Profile enrichment from provider
- 🔄 Token refresh handling
- 🚪 Single Sign-Out (SSO)
- 🛡️ Multi-Factor Authentication (MFA)
- 🔔 Login notifications
- 📊 Authentication analytics

**Security Considerations:**
- PKCE for OAuth 2.0 (prevents authorization code interception)
- State parameter validation (CSRF protection)
- Secure token storage (HttpOnly cookies)
- JWT signature verification
- Rate limiting on auth endpoints
- Suspicious login detection

**Migration Path:**
- Existing users: link OAuth accounts to local accounts
- New users: OAuth-first, optional local password
- Admin accounts: require MFA

**Definition of Done:**
- Auth0 (or equivalent) integration complete
- 5+ identity providers working
- Account linking functional
- MFA enabled for admins
- Security review passed
- Migration script for existing users
- Documentation updated

---

## Phase 4: Application Installation Wizard (Q3 2026)

### Zero-Configuration Setup Experience

**Objective:** Transform the installation process from manual configuration to a guided, interactive wizard that works for both technical and non-technical users.

**Wizard Modes:**

#### 4.1 Web-Based Wizard (Primary)
**Flow:**
1. **Welcome Screen**
   - System requirements check
   - Platform detection
   - Installation method selection (Docker, Native, Cloud)

2. **Database Setup**
   - Embedded PostgreSQL (default) vs External
   - Auto-generate secure credentials
   - Connection testing
   - Optional: Import existing database

3. **ADS-B Data Source**
   - Service selection (Airplanes.live, ADS-B Exchange, Local SDR)
   - API key configuration
   - Coverage region selection
   - Test connection

4. **Telescope Configuration**
   - Auto-discovery of ASCOM Alpaca devices on network
   - Manual entry (IP, port, device number)
   - Telescope type (Alt/Az vs Equatorial)
   - Mount-specific settings (Seestar limits, etc.)
   - Slew rate and tracking parameters

5. **Location & Limits**
   - Geographic location (GPS, IP geolocation, or manual)
   - Time zone auto-detection
   - Altitude limits (20°-80° for Seestar)
   - Collection regions (radius or polygon)

6. **User Account**
   - Create admin account
   - Email verification (optional)
   - Auth provider selection (local or OAuth)

7. **Installation**
   - Progress indicator
   - Service startup
   - Health checks
   - Error handling with retry

8. **Completion**
   - Success confirmation
   - Quick start guide
   - Link to web interface
   - System diagnostics summary

#### 4.2 CLI Wizard (Alternative)
- Terminal-based interactive prompts
- Same workflow as web wizard
- Suitable for SSH-only environments
- Uses bubble-tea or TermGL for UI

#### 4.3 Configuration Templates
**Pre-configured Templates:**
- **Seestar S50 + Airplanes.live** (Beginner)
- **Seestar S30 + Local SDR** (Intermediate)
- **Custom Alpaca + ADS-B Exchange** (Advanced)
- **Multi-telescope Setup** (Advanced)

**Features:**
- 🔍 Auto-discovery of devices and services
- 🧪 Real-time validation and testing
- 💾 Configuration export/import
- 🔄 Reconfiguration wizard (change settings post-install)
- 📋 Setup logs and diagnostics
- 🆘 Troubleshooting assistant
- 📖 Context-sensitive help
- 🌐 Multi-language support

**Technical Implementation:**
- Web wizard: React/Vue SPA served on initial startup
- Backend API: `/api/wizard/*` endpoints
- State machine for wizard flow
- Rollback on failure
- Configuration validation
- Service orchestration
- Health check integration

**Definition of Done:**
- Web wizard fully functional
- CLI wizard implemented
- 3+ configuration templates available
- Auto-discovery working for common devices
- Zero-knowledge install successful (user testing)
- Wizard can recover from errors
- Documentation includes video walkthrough
- Success rate >95% in testing

---

## Phase 5: Advanced Features (Q4 2026+)

### Additional Enhancements

#### 5.1 Mobile Apps
- Native iOS app (Swift/SwiftUI)
- Native Android app (Kotlin/Jetpack Compose)
- Push notifications for tracking events
- Haptic feedback for telescope control

#### 5.2 Advanced Tracking
- Machine learning trajectory prediction
- Multi-aircraft tracking (queue system)
- Automatic aircraft selection (interesting planes)
- Integration with FlightRadar24 data
- Astrophotography session recording

#### 5.3 Community Features
- Share tracking sessions publicly
- Gallery of aircraft photos
- Achievement system (badges)
- Social feed (recent captures)
- Monthly tracking statistics

#### 5.4 Hardware Integrations
- Camera control (exposure, ISO, etc.)
- Filter wheel control
- Focuser integration
- Weather station data
- Cloud detector integration

#### 5.5 Cloud Platform
- Hosted service option (SaaS)
- Remote telescope access
- Backup and sync
- Mobile app backend

---

## Development Priorities

### Must Have (P0)
1. ✅ Core telescope control
2. ✅ Database and persistence
3. ✅ Docker deployment
4. TermGL interface
5. Basic PWA with authentication

### Should Have (P1)
1. OAuth integration
2. Installation wizard
3. Multi-user RBAC
4. WebSocket real-time updates

### Nice to Have (P2)
1. Mobile apps
2. Community features
3. Advanced ML tracking
4. Cloud platform

### Future (P3)
1. Hardware integrations (camera, filter wheel)
2. Astrophotography automation
3. Time-lapse generation
4. Video streaming

---

## Technical Debt & Infrastructure

### Ongoing Tasks
- Continuous test coverage improvement (target: >90%)
- Performance optimization
- Security audits (quarterly)
- Dependency updates
- Documentation maintenance
- API versioning
- Database migration system
- Monitoring and alerting (Prometheus/Grafana)
- CI/CD pipeline enhancements
- Multi-architecture Docker builds (ARM64 for Raspberry Pi)

---

## Community & Contributions

### Open Source Goals
- MIT or Apache 2.0 license
- Welcoming contribution guidelines
- Code of conduct
- Issue templates
- PR review process
- Developer documentation
- API documentation
- Architecture decision records (ADRs)

### Outreach
- Astronomy forums and communities
- Conference presentations (AAS, amateur astronomy clubs)
- YouTube tutorials
- Blog posts and articles
- Social media presence

---

## Success Metrics

### Phase 1 (TermGL)
- User feedback: >4/5 stars
- Frame rate: >30fps on modest hardware
- Active terminal users: 100+

### Phase 2 (PWA)
- Web users: 1,000+
- Mobile installs: 500+
- Multi-user deployments: 50+

### Phase 3 (OAuth)
- OAuth sign-ups: >50% of new users
- Authentication security incidents: 0

### Phase 4 (Wizard)
- Installation success rate: >95%
- Average setup time: <15 minutes
- Support requests reduction: >50%

---

## Timeline Summary

| Phase | Feature | Target | Status |
|-------|---------|--------|--------|
| 0 | MVP Core System | Q4 2025 | ✅ Complete |
| 1 | TermGL Interface | Q1 2026 | 📋 Planned |
| 2 | PWA + Multi-user | Q2 2026 | 📋 Planned |
| 3 | OAuth/Auth0 | Q2-Q3 2026 | 📋 Planned |
| 4 | Installation Wizard | Q3 2026 | 📋 Planned |
| 5 | Advanced Features | Q4 2026+ | 💭 Future |

---

## Resources & References

### TermGL
- [TermGL GitHub](https://github.com/search?q=termgl) - Terminal graphics libraries
- [tcell](https://github.com/gdamore/tcell) - Go terminal library
- [tview](https://github.com/rivo/tview) - Rich TUI library for Go

### PWA Development
- [Progressive Web Apps](https://web.dev/progressive-web-apps/)
- [Service Workers](https://developers.google.com/web/fundamentals/primers/service-workers)
- [Web App Manifest](https://developer.mozilla.org/en-US/docs/Web/Manifest)

### Authentication
- [Auth0 Documentation](https://auth0.com/docs)
- [OAuth 2.0 RFC](https://tools.ietf.org/html/rfc6749)
- [PKCE RFC](https://tools.ietf.org/html/rfc7636)

### ASCOM Alpaca
- [Alpaca API Docs](https://ascom-standards.org/Developer/Alpaca.htm)
- [Seestar Alpaca Implementation](https://github.com/smart-underworld/seestar_alp)

---

## Feedback & Discussion

We welcome community input on this roadmap! Please share your thoughts:

- 🐛 **Bug Reports**: [GitHub Issues](https://github.com/unklstewy/ads-bscope/issues)
- 💡 **Feature Requests**: [GitHub Discussions](https://github.com/unklstewy/ads-bscope/discussions)
- 💬 **Chat**: [Discord/Slack] (TBD)
- 📧 **Email**: maintainer@ads-bscope.dev (TBD)

---

**Last Updated:** January 2026  
**Maintainer:** unklstewy  
**License:** TBD (MIT or Apache 2.0)
