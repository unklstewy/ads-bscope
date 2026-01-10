# ADS-B Scope Wiki

**Version**: 1.0.0  
**Last Updated**: January 10, 2026

## Table of Contents
- [Overview](#overview)
- [System Architecture](#system-architecture)
- [Features](#features)
- [Installation & Setup](#installation--setup)
- [Command-Line Tools](#command-line-tools)
- [Configuration](#configuration)
- [Database Schema](#database-schema)
- [Flight Plan & Prediction System](#flight-plan--prediction-system)
- [TUI Viewfinder](#tui-viewfinder)
- [Development](#development)
- [TODO](#todo)
- [Known Issues](#known-issues)

---

## Overview

**ADS-B Scope** is a Progressive Web Application that integrates ADS-B (Automatic Dependent Surveillance-Broadcast) aircraft tracking data with Seestar telescope control. The system automatically slews telescopes to track aircraft in real-time using the ASCOM Alpaca interface.

### Key Technologies
- **Language**: Go 1.25
- **Database**: PostgreSQL 16
- **Telescope Protocol**: ASCOM Alpaca
- **Target Hardware**: Seestar S30/S30-Pro/S50 telescopes
- **Mount Types**: Alt/Azimuth and Equatorial
- **Deployment**: Docker containers

### Project Goals
- Track aircraft with telescope in real-time
- Predict aircraft positions using flight plans and airways
- Support both Alt-Az and Equatorial mounts
- Handle Seestar-specific tracking limits (20-80° altitude)
- Provide interactive TUI and web interfaces

---

## System Architecture

### Database-Driven Design

The application uses a database-driven architecture with these benefits:

1. **API Rate Limiting**: Single collector respects API limits; multiple clients query the database
2. **Historical Data**: Store position history for velocity/acceleration calculations
3. **Shared State**: Multiple users/trackers share the same real-time data
4. **Improved Predictions**: Use actual position deltas instead of reported velocities

### Core Components

#### 1. Collector Service (`cmd/collector`)
Background service that continuously fetches aircraft data from ADS-B sources and stores in PostgreSQL.

**Features**:
- Respects API rate limits (configurable per source)
- Calculates observer-relative measurements (range, bearing, altitude, azimuth)
- Computes position deltas (distance, time, altitude, track)
- Calculates actual velocities from position changes
- Marks aircraft as trackable based on telescope limits
- Auto-cleanup of stale data

**Usage**:
```bash
go run cmd/collector/main.go --config configs/config.json
```

#### 2. Aircraft Repository
Data access layer providing:
- `UpsertAircraft()`: Insert/update with automatic delta calculation
- `GetTrackableAircraft()`: Get aircraft within telescope limits
- `GetAircraftByICAO()`: Retrieve specific aircraft
- `GetPositionHistory()`: Historical positions for prediction
- `UpdateTrackableStatus()`: Update trackability based on altitude

#### 3. Flight Plan Repository
Manages flight plan and waypoint data:
- `GetFlightPlanByICAO()`: Retrieve flight plan for aircraft
- `GetFlightPlanRoute()`: Get waypoint sequence
- `FindNearbyAirways()`: Query airways within radius
- `ParseRouteString()`: Parse flight plan routes

---

## Features

### ✅ Implemented Features

#### ADS-B Data Integration
- **Online Sources**: AirplanesLive API support with rate limiting (6s minimum)
- **Rate Limit Handling**: HTTP 429 detection with Retry-After header support
- **Exponential Backoff**: Automatic retry with configurable backoff (1s → 2s → 4s)
- **Local SDR**: Ready for RTL-SDR/HackRF One integration
- **Search Radius**: Configurable radius (default 100 NM)
- **Auto-refresh**: Configurable update intervals (default 10s)
- **Data Quality**: Position delta tracking for accuracy verification

#### Database & Storage
- **PostgreSQL**: Complete schema with indexing
- **Position History**: Time-series storage with delta calculations
- **Flight Plans**: NASR waypoint database (72,313 waypoints, 997 airways)
- **Historical Tracking**: Session metadata and telescope command logs
- **Auto-cleanup**: Configurable stale data removal

#### Telescope Control
- **ASCOM Alpaca**: REST API integration
- **Mount Types**: Both Alt-Az and Equatorial support
- **Slew Control**: Configurable slew rates
- **Tracking Limits**: Automatic enforcement based on mount type
- **Seestar-Specific**:
  - Altitude range: 20-80° (Alt-Az), 15-85° (Equatorial)
  - Field rotation awareness near zenith
  - 360° azimuth rotation capability

#### Prediction System (4 Phases Complete)
- **Phase 1**: NASR waypoint database integration
- **Phase 2**: FlightAware API integration (DISABLED - cost prohibitive)
- **Phase 3**: Waypoint-based prediction with great circle interpolation
- **Phase 4**: Airway-based prediction with track matching

**Prediction Cascade**:
1. **Waypoint Prediction** (95% confidence): Uses filed flight plan (requires FlightAware)
2. **Airway Prediction** (90% confidence): Matches to Victor/Jet routes
3. **Dead Reckoning** (decreasing confidence): Simple velocity extrapolation

**Note**: FlightAware API is currently disabled due to cost. Only airway-based and dead reckoning predictions are active.

#### TUI Viewfinder
Interactive text-based aircraft tracking display with:
- **Sky View**: 80x30 character grid showing aircraft positions
- **Range Rings**: Color-coded distance indicators (5, 10, 25, 50 NM)
- **Prediction Modes**: Visual indicators ([WPT], [AWY], [DR])
- **Flight Plans**: Departure, arrival, next waypoint display
- **Zoom Controls**: 0.5x to 4.0x magnification
- **Velocity Vectors**: Arrows showing aircraft heading/speed
- **Track Trails**: 10-position breadcrumb history
- **Interactive Selection**: Keyboard navigation and tracking
- **Legend Panel**: Comprehensive symbol reference

#### Coordinate Transformations
- **Geographic → Horizontal**: Lat/Lon/Alt → Alt/Az
- **Horizontal → Equatorial**: Alt/Az → RA/Dec (for equatorial mounts)
- **Great Circle Navigation**: Spherical trigonometry for waypoint routing
- **Cross-track Distance**: Perpendicular distance to flight path
- **Time-to-closest**: Approach detection and ETA calculation

#### Configuration Management
- **Centralized Config**: Single JSON file (`configs/config.json`)
- **Environment Override**: Sensitive data via env vars
- **Validation**: Runtime configuration checking
- **Documentation**: Complete configuration guide

---

## Installation & Setup

### Prerequisites
- Docker Desktop (or Docker + Docker Compose)
- Go 1.25+ (for local development)
- PostgreSQL 16 (handled by Docker)
- Observer location (latitude/longitude/elevation)
- Seestar telescope IP address (optional for testing)

### Quick Start with Docker

1. **Clone repository**
   ```bash
   git clone https://github.com/unklstewy/ads-bscope.git
   cd ads-bscope
   ```

2. **Configure environment**
   ```bash
   cp .env.example .env
   # Edit .env and set DB_PASSWORD
   nano .env
   ```

3. **Configure location and telescope**
   ```bash
   nano configs/config.json
   # Set observer latitude, longitude, elevation
   # Set telescope base_url if available
   ```

4. **Start services**
   ```bash
   docker-compose up -d
   ```

5. **Verify database**
   ```bash
   docker-compose logs -f adsbscope-db
   docker-compose logs -f ads-bscope
   ```

6. **Access application**
   - Web Interface: http://localhost:8080 (when implemented)
   - Database: localhost:5432 (username: adsbscope)

### Local Development Setup

1. **Install Go**
   ```bash
   brew install go
   go version  # Should be 1.25+
   ```

2. **Start PostgreSQL**
   ```bash
   docker-compose up -d adsbscope-db
   ```

3. **Set environment variables**
   ```bash
   export CONFIG_PATH=configs/config.json
   export ADS_BSCOPE_DB_PASSWORD=changeme
   ```

4. **Run collector**
   ```bash
   go run cmd/collector/main.go
   ```

5. **Run TUI viewfinder** (in another terminal)
   ```bash
   go run cmd/tui-viewfinder/main.go
   ```

---

## Command-Line Tools

### Data Collection

#### `cmd/collector`
Continuous background service that fetches aircraft data and populates database.

```bash
go run cmd/collector/main.go --config configs/config.json
```

**Features**:
- API rate limiting
- Position delta calculation
- Observer-relative measurements
- Automatic trackability marking
- Stale data cleanup

---

### Testing & Verification

#### `cmd/test-adsb`
Test ADS-B data source connectivity.

```bash
go run cmd/test-adsb/main.go
```

**Tests**:
- API connectivity
- Data parsing
- Geographic coordinate handling
- Response time measurement

#### `cmd/test-api-rate`
Verify API rate limiting is working correctly.

```bash
go run cmd/test-api-rate/main.go
```

**Validates**:
- Minimum time between requests
- Request queueing
- Rate limit configuration

---

### Aircraft Tracking

#### `cmd/track-aircraft`
Direct aircraft tracking without database (legacy).

```bash
go run cmd/track-aircraft/main.go --icao A12345
```

**Features**:
- Direct API queries
- Real-time position updates
- Coordinate transformations
- Telescope slew commands

#### `cmd/track-aircraft-db`
Database-driven aircraft tracking with advanced prediction.

```bash
go run cmd/track-aircraft-db/main.go --icao AAL123
```

**Features**:
- Database queries (no API rate limits)
- Flight plan integration
- Waypoint-based prediction
- Airway matching
- Dead reckoning fallback
- Confidence scoring

---

### TUI Viewfinder

#### `cmd/tui-viewfinder`
Interactive text-based aircraft tracking display.

```bash
go run cmd/tui-viewfinder/main.go
```

**Controls**:
- `↑/↓` or `k/j`: Navigate aircraft list
- `ENTER` or `SPACE`: Track selected aircraft
- `S`: Stop tracking
- `+` or `=`: Zoom in (max 4.0x)
- `-` or `_`: Zoom out (min 0.5x)
- `0`: Reset zoom to 1.0x
- `Q`: Quit

**Display Elements**:
- Sky view with aircraft positions
- Telescope crosshair (+)
- Velocity vectors (→)
- Track trails (· breadcrumbs)
- Range rings (◦ at 5/10/25/50 NM)
- Prediction mode indicators
- Flight plan information
- Legend panel

---

### Flight Plan Management

#### `cmd/import-nasr`
Import FAA NASR (National Airspace System Resources) data.

```bash
go run cmd/import-nasr/main.go --nasr-path data/nasr
```

**Imports**:
- Waypoints: 72,313 navigation fixes, VORs, NDBs
- Airways: 997 unique Victor/Jet routes with sequences
- Data source: FAA 28-day NASR subscription

**Data Files**:
- `FIX.txt`: Navigation fixes
- `NAV.txt`: VOR/NDB navaids
- `AWY.txt`: Airway definitions

#### `cmd/verify-nasr`
Verify NASR import was successful.

```bash
go run cmd/verify-nasr/main.go
```

**Checks**:
- Waypoint count and types
- Airway definitions and sequences
- Database integrity
- Spatial queries

#### `cmd/fetch-flightplans`
Fetch flight plans from FlightAware API.

```bash
go run cmd/fetch-flightplans/main.go
```

**Features**:
- Auto-fetch for tracked aircraft
- 10 requests/hour rate limit
- Route string parsing
- Waypoint resolution
- 60-minute refresh interval

#### `cmd/verify-flightplans`
Test FlightAware integration and display flight plans.

```bash
go run cmd/verify-flightplans/main.go --callsign UAL1
```

**Displays**:
- Departure/arrival airports
- Filed route string
- Resolved waypoint sequence
- ETAs and passed waypoints

---

## Configuration

### Configuration File Structure

**Location**: `configs/config.json`

```json
{
  "server": {
    "port": "8080",
    "host": "0.0.0.0"
  },
  "database": {
    "driver": "postgres",
    "host": "localhost",
    "port": 5432,
    "database": "adsbscope",
    "username": "adsbscope",
    "password": "",
    "ssl_mode": "disable",
    "max_open_conns": 25,
    "max_idle_conns": 5
  },
  "telescope": {
    "base_url": "http://seestar.local:11111",
    "device_number": 0,
    "mount_type": "altaz",
    "model": "seestar-s30",
    "slew_rate": 3.0,
    "supports_meridian_flip": false,
    "max_altitude": 80,
    "min_altitude": 20
  },
  "adsb": {
    "sources": [{
      "name": "airplanes.live",
      "type": "airplanes.live",
      "enabled": true,
      "base_url": "https://api.airplanes.live/v2",
      "rate_limit_seconds": 6.0
    }],
    "search_radius_nm": 100.0,
    "update_interval_seconds": 10
  },
  "observer": {
    "latitude": 35.7796,
    "longitude": -78.6382,
    "elevation": 100.0,
    "timezone": "America/New_York"
  },
  "flightaware": {
    "api_key": "",
    "auto_fetch": true,
    "refresh_interval_minutes": 60
  }
}
```

### Environment Variables

Override configuration with environment variables (recommended for secrets):

```bash
# Configuration file path
export CONFIG_PATH=configs/config.json

# Database
export ADS_BSCOPE_DB_PASSWORD=secure_password

# ADS-B (if using authenticated services)
export ADS_BSCOPE_ADSB_API_KEY=your_api_key

# Telescope
export ADS_BSCOPE_TELESCOPE_URL=http://192.168.1.100:11111

# FlightAware
export FLIGHTAWARE_API_KEY=your_flightaware_key
```

### Telescope Altitude Limits

**Seestar Alt-Az Mount**:
- Min: 20° (atmospheric refraction and practical limits)
- Max: 80° (field rotation becomes severe)
- Above 85°: Telescope may stop stacking frames

**Seestar Equatorial (with wedge)**:
- Min: 15° (atmospheric limit)
- Max: 85° (physical limit)
- No field rotation issues

**Field Rotation Notes**:
- Most severe near zenith and when pointing N/S
- Minimal when pointing E/W
- EQ mode eliminates field rotation entirely

### ADS-B Data Sources

**Supported Sources**:
- `airplanes.live`: Free API with 6-second rate limit
- Local SDR: RTL-SDR, HackRF One (via dump1090/readsb)

**Configuration**:
```json
"adsb": {
  "sources": [{
    "name": "airplanes.live",
    "type": "airplanes.live",
    "enabled": true,
    "base_url": "https://api.airplanes.live/v2",
    "rate_limit_seconds": 6.0
  }],
  "search_radius_nm": 100.0,
  "update_interval_seconds": 10
}
```

---

## Database Schema

### Core Tables

#### `aircraft`
Current state of all tracked aircraft.

```sql
CREATE TABLE aircraft (
    icao TEXT PRIMARY KEY,
    callsign TEXT,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude_ft DOUBLE PRECISION,
    ground_speed_kts DOUBLE PRECISION,
    track_deg DOUBLE PRECISION,
    vertical_rate_fpm DOUBLE PRECISION,
    first_seen TIMESTAMP NOT NULL,
    last_seen TIMESTAMP NOT NULL,
    last_updated TIMESTAMP NOT NULL,
    position_count INTEGER DEFAULT 1,
    
    -- Observer-relative (pre-calculated)
    range_nm DOUBLE PRECISION,
    bearing_deg DOUBLE PRECISION,
    altitude_deg DOUBLE PRECISION,  -- Elevation angle
    azimuth_deg DOUBLE PRECISION,
    
    -- Approach tracking
    is_approaching BOOLEAN DEFAULT FALSE,
    closest_range_nm DOUBLE PRECISION,
    eta_closest_seconds INTEGER,
    
    -- Status
    is_visible BOOLEAN DEFAULT TRUE,
    is_trackable BOOLEAN DEFAULT FALSE,
    last_trackable TIMESTAMP
);
```

#### `aircraft_positions`
Time-series position history with calculated deltas.

```sql
CREATE TABLE aircraft_positions (
    id BIGSERIAL PRIMARY KEY,
    icao TEXT NOT NULL REFERENCES aircraft(icao),
    timestamp TIMESTAMP NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude_ft DOUBLE PRECISION,
    ground_speed_kts DOUBLE PRECISION,
    track_deg DOUBLE PRECISION,
    vertical_rate_fpm DOUBLE PRECISION,
    
    -- Calculated deltas
    delta_time_seconds DOUBLE PRECISION,
    delta_distance_nm DOUBLE PRECISION,
    delta_altitude_ft DOUBLE PRECISION,
    delta_track_deg DOUBLE PRECISION,
    
    -- Actual velocities (more accurate than reported)
    actual_speed_kts DOUBLE PRECISION,
    actual_vertical_rate_fpm DOUBLE PRECISION,
    
    -- Observer-relative
    range_nm DOUBLE PRECISION,
    altitude_angle_deg DOUBLE PRECISION,
    azimuth_deg DOUBLE PRECISION
);
```

### Flight Plan Tables

#### `waypoints`
Navigation fixes, VORs, NDBs, and airports (72,313 records).

```sql
CREATE TABLE waypoints (
    id SERIAL PRIMARY KEY,
    identifier TEXT NOT NULL,
    name TEXT,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    type TEXT NOT NULL,  -- fix, vor, ndb, airport, intersection
    region TEXT,
    description TEXT,
    UNIQUE(identifier, region)
);
```

#### `airways`
Victor airways, Jet routes, RNAV routes (997 unique airways).

```sql
CREATE TABLE airways (
    id SERIAL PRIMARY KEY,
    identifier TEXT NOT NULL,  -- e.g., "J121", "V1"
    type TEXT NOT NULL,        -- victor, jet, rnav, other
    sequence INTEGER NOT NULL,
    waypoint_id INTEGER REFERENCES waypoints(id),
    min_altitude INTEGER,
    max_altitude INTEGER,
    direction TEXT,
    UNIQUE(identifier, sequence)
);
```

#### `flight_plans`
Filed or retrieved flight plans.

```sql
CREATE TABLE flight_plans (
    id SERIAL PRIMARY KEY,
    icao TEXT NOT NULL,
    callsign TEXT NOT NULL,
    departure_icao TEXT,
    arrival_icao TEXT,
    route TEXT,
    filed_altitude INTEGER,
    aircraft_type TEXT,
    filed_time TIMESTAMP,
    etd TIMESTAMP,
    eta TIMESTAMP,
    last_updated TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_flight_plan_aircraft 
        FOREIGN KEY (icao) REFERENCES aircraft(icao)
);
```

#### `flight_plan_routes`
Resolved waypoint sequences from flight plans.

```sql
CREATE TABLE flight_plan_routes (
    id SERIAL PRIMARY KEY,
    flight_plan_id INTEGER NOT NULL 
        REFERENCES flight_plans(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    waypoint_id INTEGER NOT NULL 
        REFERENCES waypoints(id),
    eta TIMESTAMP,
    passed BOOLEAN DEFAULT FALSE,
    UNIQUE(flight_plan_id, sequence)
);
```

### Tracking Tables

#### `tracking_sessions`
Metadata about data collection sessions.

```sql
CREATE TABLE tracking_sessions (
    id SERIAL PRIMARY KEY,
    start_time TIMESTAMP NOT NULL DEFAULT NOW(),
    end_time TIMESTAMP,
    observer_latitude DOUBLE PRECISION,
    observer_longitude DOUBLE PRECISION,
    observer_elevation DOUBLE PRECISION,
    adsb_source TEXT,
    telescope_model TEXT,
    notes TEXT
);
```

#### `telescope_tracking_log`
Log of telescope tracking commands.

```sql
CREATE TABLE telescope_tracking_log (
    id BIGSERIAL PRIMARY KEY,
    icao TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Aircraft state
    aircraft_latitude DOUBLE PRECISION NOT NULL,
    aircraft_longitude DOUBLE PRECISION NOT NULL,
    aircraft_altitude_ft DOUBLE PRECISION,
    aircraft_range_nm DOUBLE PRECISION,
    
    -- Telescope command
    telescope_altitude_deg DOUBLE PRECISION NOT NULL,
    telescope_azimuth_deg DOUBLE PRECISION NOT NULL,
    mount_type TEXT NOT NULL,
    
    -- Tracking result
    command_sent BOOLEAN DEFAULT FALSE,
    command_success BOOLEAN,
    error_message TEXT,
    
    -- Prediction info
    predicted_position BOOLEAN DEFAULT FALSE,
    prediction_latency_seconds DOUBLE PRECISION,
    prediction_confidence DOUBLE PRECISION
);
```

---

## Flight Plan & Prediction System

### Overview

Four-phase enhanced prediction system providing intelligent aircraft position forecasting.

### Phase 1: NASR Waypoint Database

**Imported Data**:
- 72,313 waypoints (fixes, VORs, NDBs, TACANs)
- 997 unique airways (Victor, Jet, RNAV)
- Complete US airspace structure

**Import Process**:
```bash
# Download NASR from FAA (28-day subscription)
# Extract to data/nasr/

# Import waypoints and airways
go run cmd/import-nasr/main.go --nasr-path data/nasr

# Verify import
go run cmd/verify-nasr/main.go
```

**Files Used**:
- `FIX.txt`: Navigation fixes
- `NAV.txt`: VOR/NDB navaids  
- `AWY.txt`: Airway route definitions

### Phase 2: FlightAware API Integration

**API Configuration**:
```json
"flightaware": {
  "api_key": "your_key_here",
  "auto_fetch": true,
  "refresh_interval_minutes": 60
}
```

**Rate Limits**:
- 10 requests/hour
- Automatic queuing
- 60-minute refresh interval

**Fetcher Service**:
```bash
# Auto-fetch flight plans for tracked aircraft
go run cmd/fetch-flightplans/main.go

# Test specific callsign
go run cmd/verify-flightplans/main.go --callsign UAL1
```

**Route Parsing**:
- Parses SID, waypoints, airways, STAR
- Resolves waypoints from NASR database
- Creates waypoint sequence with ETAs

### Phase 3: Waypoint-Based Prediction

**Algorithm**:
1. Determine which waypoints aircraft has passed
2. Identify next waypoint in sequence
3. Calculate great circle route to next waypoint
4. Interpolate position along route using slerp (spherical linear interpolation)

**Confidence**: 95% (high confidence with known route)

**Usage**:
```go
predicted := tracking.PredictPositionWithWaypoints(
    aircraft,
    waypointList,
    predictionTime,
)
```

**Features**:
- Great circle navigation
- Spherical trigonometry
- Cross-track error detection
- Automatic waypoint progression

### Phase 4: Airway-Based Prediction

**Algorithm**:
1. Query nearby airways (25 NM radius)
2. Filter by altitude (Victor <18k ft, Jet ≥18k ft)
3. Score airways by:
   - Track alignment (70%): Match aircraft heading to airway bearing
   - Distance to centerline (30%): Perpendicular distance
4. Select best match (score >0.6)
5. Predict along airway centerline

**Confidence**: 90% (good confidence when on airway)

**Usage**:
```go
// Find nearby airways
airways := fpRepo.FindNearbyAirways(ctx, lat, lon, radius, minAlt, maxAlt)

// Filter by altitude
airways = tracking.FilterAirwaysByAltitude(airways, altitudeFt)

// Match best airway
matched := tracking.MatchAirway(aircraft, airways)

// Predict position
predicted := tracking.PredictPositionWithAirway(aircraft, matched, predictionTime)
```

### Prediction Cascade

Three-tier fallback system:

1. **Waypoint Prediction** (if flight plan available)
   - 95% confidence
   - Uses filed flight plan
   - Most accurate

2. **Airway Prediction** (if no flight plan but near airway)
   - 90% confidence
   - Matches to Victor/Jet routes
   - Good for IFR traffic

3. **Dead Reckoning** (fallback)
   - Decreasing confidence (1.0 at 0s → 0.0 at 60s)
   - Simple velocity extrapolation
   - Last resort

**Automatic Selection**:
```go
// System automatically selects best method
if len(waypointList) > 0 {
    // Use waypoint prediction
} else if nearbyAirways := findAirways(); len(nearbyAirways) > 0 {
    // Use airway prediction
} else {
    // Fall back to dead reckoning
}
```

---

## TUI Viewfinder

### Overview

Interactive text-based aircraft tracking display with real-time visualization.

### Launch

```bash
go run cmd/tui-viewfinder/main.go
```

### Display Layout

```
┌─────────────────────────────────────────────────────┐
│                   SKY VIEW (80x30)                  │  Legend
│                                                     │  ─────────
│  N                   E         S         W          │  ○ Untracked
│                     ●                               │  ● Selected
│         ◉→                                          │  ◉ Tracking
│                                                     │  + Telescope
│              ○                                      │  · Trail/Ring
│   ·····                                             │  → Velocity
│      +                                              │
│         ····· ○→                                    │  Prediction
│                                                     │  ─────────
│                                                     │  [WPT] Waypoint
│                           ○                         │  [AWY] Airway
│                                                     │  [DR]  Dead Reckon
│  ················································   │
└─────────────────────────────────────────────────────┘  Range Rings
                                                          ─────────
Trackable Aircraft: (3)                                   ◦  5 nm
                                                          ◦ 10 nm
→ AAL123     35000 ft   45.2 nm  Az:120° Alt:45°  12s [WPT] [TRACKING]  ◦ 25 nm
    Plan: KJFK → KLAX (next: CHSLY)                      ◦ 50 nm
  UAL456     38000 ft   52.1 nm  Az:200° Alt:50°  25s [AWY:J121]
  DAL789     32000 ft   68.5 nm  Az:310° Alt:38°  43s [DR]

Telescope: Az 120.5°  Alt 45.2°  Zoom: 1.0x

↑/↓: Select  ENTER/SPACE: Track  S: Stop  +/-: Zoom  0: Reset  Q: Quit
```

### Controls

| Key | Action |
|-----|--------|
| `↑` or `k` | Select previous aircraft |
| `↓` or `j` | Select next aircraft |
| `ENTER` or `SPACE` | Track selected aircraft |
| `S` | Stop tracking |
| `+` or `=` | Zoom in (max 4.0x) |
| `-` or `_` | Zoom out (min 0.5x) |
| `0` | Reset zoom to 1.0x |
| `Q` | Quit |

### Display Elements

#### Sky View
- **Grid**: 80x30 character display
- **Coordinate System**: Azimuth (X) × Altitude (Y)
- **Horizon Line**: Dotted line at ~80% from top
- **Cardinal Directions**: N, E, S, W markers

#### Aircraft Symbols
- `○` Untracked aircraft (light blue)
- `●` Selected aircraft (yellow)
- `◉` Tracked aircraft (green, bold)
- `+` Telescope crosshair (orange)

#### Velocity Vectors
- `→` Arrow showing direction of motion
- Length proportional to ground speed
- Only shown for aircraft >50 knots

#### Track Trails
- `·` Breadcrumb dots showing past positions
- 10-position history per aircraft
- Fades as aircraft moves

#### Range Rings
- `◦` Partial arc segments
- Displayed when aircraft near 5, 10, 25, or 50 NM
- Color-coded by range

#### Prediction Mode Indicators
- `[WPT]` Waypoint-based prediction (95% confidence)
- `[AWY:J121]` Airway prediction with airway ID (90% confidence)
- `[DR]` Dead reckoning (decreasing confidence)
- Automatically displayed when data >30s old

#### Flight Plan Display
- Shows for selected aircraft only
- Format: `Plan: KDEP → KARR (next: WAYPOINT)`
- Displays departure, arrival, next waypoint

#### Legend Panel
- Comprehensive symbol reference
- Prediction mode explanations
- Range ring distances with colors

### Zoom Functionality

**Zoom Levels**: 0.5x to 4.0x
- **1.0x**: Normal view (full altitude range)
- **2.0x**: 2× magnification (half altitude range visible)
- **4.0x**: 4× magnification (quarter altitude range visible)

**Effect**: Higher zoom shows smaller altitude range in more detail

### Technical Details

**Update Rate**: 2 seconds
**Position Source**: Database queries (no API rate limits)
**Prediction Threshold**: 30 seconds (stale data triggers prediction)
**Trail Length**: 10 positions (approx 20 seconds)

---

## Development

### Project Structure

```
ads-bscope/
├── cmd/                    # Command-line applications
│   ├── ads-bscope/        # Main web application (TODO)
│   ├── collector/         # Background data collector
│   ├── fetch-flightplans/ # FlightAware fetcher
│   ├── import-nasr/       # NASR data importer
│   ├── test-adsb/         # ADS-B connectivity test
│   ├── test-api-rate/     # Rate limit test
│   ├── track-aircraft/    # Direct tracking (legacy)
│   ├── track-aircraft-db/ # Database-driven tracking
│   ├── tui-viewfinder/    # TUI application
│   ├── verify-flightplans/# FlightAware test
│   └── verify-nasr/       # NASR import verification
├── pkg/                   # Public library code
│   ├── adsb/             # ADS-B data sources
│   ├── alpaca/           # ASCOM Alpaca client
│   ├── config/           # Configuration management
│   ├── coordinates/      # Coordinate transformations
│   ├── flightaware/      # FlightAware API client
│   └── tracking/         # Tracking algorithms & prediction
├── internal/             # Private application code
│   ├── api/             # HTTP API handlers (TODO)
│   ├── auth/            # Authentication (TODO)
│   └── db/              # Database layer
├── web/                 # PWA frontend (TODO)
├── configs/             # Configuration files
├── data/                # Data files (NASR, etc.)
├── docs/                # Documentation
├── scripts/             # Build/deployment scripts (TODO)
└── test/                # Integration tests (TODO)
```

### Code Standards

**Commenting Requirements** (from WARP.md):
- Every function, method, interface, and struct must have comprehensive documentation
- Explain the "why" not just the "what"
- Document expected input/output ranges and units (especially coordinates)
- Include references to algorithms or external documentation

**Units and Conventions**:
- Angles: Degrees for display, radians for calculations
- Coordinates: Document J2000 vs JNOW vs apparent
- Time: UTC for all astronomical calculations
- Distances: Nautical miles for aviation, meters for internal

**Testing**:
- Use ASCOM Alpaca Simulator for telescope testing
- Integration tests for database operations
- Unit tests for coordinate transformations
- End-to-end tests for prediction algorithms

### Development Commands

```bash
# Build all binaries
go build ./cmd/...

# Run tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Format code
go fmt ./...

# Vet code
go vet ./...

# Lint (requires golangci-lint)
golangci-lint run

# Tidy dependencies
go mod tidy
```

### Docker Development

```bash
# Build image
docker build -t ads-bscope:latest .

# Run container
docker run -p 8080:8080 ads-bscope:latest

# Docker Compose
docker-compose up -d      # Start services
docker-compose down       # Stop services
docker-compose logs -f    # View logs
docker-compose ps         # List services
```

### Database Management

```bash
# Connect to PostgreSQL
docker exec -it adsbscope-db psql -U adsbscope -d adsbscope

# Backup database
docker exec adsbscope-db pg_dump -U adsbscope adsbscope > backup.sql

# Restore database
docker exec -i adsbscope-db psql -U adsbscope adsbscope < backup.sql

# Reset database
docker-compose down -v
docker-compose up -d
```

### Adding New ADS-B Sources

1. Implement `adsb.DataSource` interface in `pkg/adsb/`
2. Add configuration in `pkg/config/config.go`
3. Register source in collector service
4. Add rate limiting configuration
5. Document in `configs/README.md`

### Adding New Prediction Methods

1. Implement prediction function in `pkg/tracking/prediction.go`
2. Add confidence scoring algorithm
3. Integrate into prediction cascade
4. Add unit tests
5. Document in `docs/PREDICTION.md`

---

## TODO

### High Priority

#### Web Interface (PWA)
- [ ] React/Vue frontend framework setup
- [ ] WebSocket connection for real-time updates
- [ ] Interactive sky map (2D/3D)
- [ ] Aircraft selection and tracking controls
- [ ] Flight plan visualization
- [ ] User authentication system
- [ ] Role-based access control
- [ ] Mobile-responsive design
- [ ] Offline capability (PWA features)

#### Telescope Integration
- [ ] Complete ASCOM Alpaca client implementation
- [ ] Telescope connection management
- [ ] Slew command execution
- [ ] Position feedback loop
- [ ] Meridian flip handling (for GEM mounts)
- [ ] Focus control integration
- [ ] Camera control (Seestar-specific)
- [ ] Mount calibration/alignment

#### Advanced Tracking
- [ ] Multi-aircraft tracking queue
- [ ] Automatic aircraft selection (priority algorithm)
- [ ] Collision avoidance (multiple trackers)
- [ ] Schedule-based tracking (specific flights/times)
- [ ] Track recording and playback
- [ ] Image capture automation

#### Local SDR Support
- [ ] RTL-SDR integration (dump1090)
- [ ] HackRF One support
- [ ] readsb integration
- [ ] Beast protocol parser
- [ ] Local antenna pattern optimization
- [ ] Multi-receiver support (MLAT)

### Medium Priority

#### Data Analysis & Visualization
- [ ] Historical track playback
- [ ] Heatmaps of traffic patterns
- [ ] Statistics dashboard
- [ ] Flight profile analysis
- [ ] Performance metrics (tracking accuracy)
- [ ] API usage analytics

#### Enhanced Predictions
- [ ] Weather data integration (winds aloft)
- [ ] METAR/TAF parsing for conditions
- [ ] Wind correction in predictions
- [ ] Turbulence detection
- [ ] Deviation alerts (off-route aircraft)
- [ ] Machine learning for pattern recognition

#### Database Optimizations
- [ ] Time-series database (TimescaleDB)
- [ ] Data retention policies
- [ ] Archival system
- [ ] Query optimization
- [ ] Caching layer (Redis)
- [ ] Read replicas for scaling

#### Configuration & Deployment
- [ ] Configuration UI (web-based)
- [ ] Setup wizard for first-time users
- [ ] Health check endpoints
- [ ] Prometheus metrics export
- [ ] Grafana dashboards
- [ ] Kubernetes deployment manifests
- [ ] Helm charts

### Low Priority

#### Additional Features
- [ ] Multi-observer support (network of trackers)
- [ ] Flight notifications (specific aircraft/routes)
- [ ] Email/SMS alerts
- [ ] Integration with flight tracking websites
- [ ] Export tracking data (KML, GPX)
- [ ] API for third-party integrations
- [ ] Mobile app (iOS/Android)

#### Documentation
- [ ] API documentation (OpenAPI/Swagger)
- [ ] Video tutorials
- [ ] Architecture diagrams
- [ ] Performance benchmarks
- [ ] Troubleshooting guide
- [ ] FAQ section

#### Testing & Quality
- [ ] Integration test suite
- [ ] End-to-end tests (Playwright/Cypress)
- [ ] Load testing
- [ ] Security audit
- [ ] Accessibility testing (WCAG)
- [ ] Cross-browser testing
- [ ] CI/CD pipeline (GitHub Actions)

---

## Known Issues

### Critical

**None at this time.**

### Major

#### Issue #1: FlightAware API Disabled Due to Cost
**Status**: Active  
**Description**: FlightAware API disabled due to cost constraints (500 req/month on free tier insufficient, paid tiers expensive).  
**Impact**: Waypoint-based prediction not available. System falls back to airway matching or dead reckoning.  
**Workaround**: Use airway-based prediction for IFR traffic.  
**Fix Plan**: 
- Explore free alternative flight plan sources (FAA SWIM, ADS-B Exchange)
- Implement manual flight plan entry
- Consider selective enablement for high-priority aircraft

#### Issue #2: Rate Limit Handling Not Tested
**Status**: Active  
**Description**: HTTP 429 rate limit handling implemented but not tested against actual rate limits.  
**Impact**: Unknown if retry logic works correctly in production.  
**Workaround**: None needed - graceful degradation.  
**Fix Plan**: 
- Temporarily reduce rate limits to trigger 429 responses
- Verify Retry-After header parsing
- Test exponential backoff behavior
- Validate rate limit header extraction

#### Issue #3: No Hardware Telescope Testing
**Status**: Active  
**Description**: Telescope control has not been tested with actual Seestar hardware.  
**Impact**: Unknown if telescope commands work correctly in practice.  
**Workaround**: Use ASCOM Alpaca Simulator for development.  
**Fix Plan**: 
- Acquire Seestar S30/S50 for testing
- Verify slewstar_alp integration
- Test tracking accuracy and field rotation handling

### Minor

#### Issue #3: TUI Performance with Many Aircraft
**Status**: Active  
**Description**: TUI viewfinder may become sluggish with 50+ aircraft.  
**Impact**: Reduced user experience in busy airspace.  
**Workaround**: Reduce search radius or filter by altitude.  
**Fix Plan**:
- Implement virtual scrolling
- Limit displayed aircraft (top 20 by range/priority)
- Optimize rendering pipeline

#### Issue #4: Prediction Confidence Not Calibrated
**Status**: Active  
**Description**: Confidence scores are theoretical, not validated against real flight data.  
**Impact**: Users may over/under-trust predictions.  
**Workaround**: None (informational only).  
**Fix Plan**:
- Collect ground truth data
- Calculate actual prediction errors
- Calibrate confidence scoring

#### Issue #5: No Automatic NASR Updates
**Status**: Active  
**Description**: NASR data must be manually downloaded and imported every 28 days.  
**Impact**: Airways and waypoints may become outdated.  
**Workaround**: Manual import every month.  
**Fix Plan**:
- Implement automatic NASR download (requires FAA subscription)
- Schedule automatic imports
- Version tracking for NASR cycles

#### Issue #6: Database Cleanup Not Configurable
**Status**: Active  
**Description**: Stale data cleanup uses hardcoded thresholds (5 minutes).  
**Impact**: May delete data too quickly in low-traffic scenarios.  
**Workaround**: None (acceptable for most use cases).  
**Fix Plan**:
- Add cleanup configuration to config.json
- Make thresholds configurable per use case
- Add manual cleanup command

### Cosmetic

#### Issue #7: TUI Color Scheme Not Customizable
**Status**: Active  
**Description**: TUI uses hardcoded colors that may not work with all terminal themes.  
**Impact**: Reduced readability in some terminals.  
**Workaround**: Use terminal with dark background.  
**Fix Plan**:
- Add color scheme configuration
- Detect terminal capabilities
- Provide presets (dark/light/high-contrast)

#### Issue #8: No Progress Indicators
**Status**: Active  
**Description**: Long-running operations (NASR import) lack progress feedback.  
**Impact**: User uncertainty about operation status.  
**Workaround**: Check database directly.  
**Fix Plan**:
- Add progress bars to import commands
- Show record counts during processing
- Implement verbose mode

---

## Contributing

### Getting Started

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Follow code standards (see Development section)
4. Add tests for new functionality
5. Update documentation (wiki, comments, README)
6. Commit changes (`git commit -m 'Add amazing feature'`)
7. Push to branch (`git push origin feature/amazing-feature`)
8. Open Pull Request

### Commit Message Format

Use conventional commits:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `refactor:` Code refactoring
- `test:` Test additions/changes
- `chore:` Maintenance tasks

Example:
```
feat(telescope): add meridian flip support

Implement automatic meridian flip detection and execution for
German Equatorial Mounts (GEM) when tracking past meridian.

- Add flip detection algorithm
- Handle flip timing and execution
- Update tracking limits accordingly
- Add configuration option for flip behavior

Closes #42
```

### Code Review Checklist

- [ ] Code follows project conventions
- [ ] Comments explain "why" not just "what"
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] No breaking changes (or clearly documented)
- [ ] Passes CI checks
- [ ] Performance impact considered
- [ ] Security implications reviewed

---

## References

### External Documentation

- [ASCOM Alpaca API](https://ascom-standards.org/Developer/Alpaca.htm)
- [Seestar Alpaca Implementation](https://github.com/smart-underworld/seestar_alp)
- [AirplanesLive API](https://api.airplanes.live/)
- [FAA NASR Subscription](https://www.faa.gov/air_traffic/flight_info/aeronav/aero_data/NASR_Subscription/)
- [FlightAware API](https://www.flightaware.com/commercial/aeroapi/)

### Internal Documentation

- [Configuration Guide](configs/README.md)
- [Database Architecture](docs/DATABASE_ARCHITECTURE.md)
- [NASR Import Guide](docs/NASR_IMPORT.md)
- [FlightAware Integration](docs/FLIGHTAWARE.md)
- [Airway Prediction](docs/AIRWAY_PREDICTION.md)
- [Development Guide](WARP.md)

### Academic References

**Spherical Trigonometry**:
- Vincenty's formulae for geodesic calculations
- Great circle navigation algorithms
- Haversine distance formula

**Astronomical Coordinates**:
- USNO Circular 179: The IAU Resolutions on Astronomical Reference Systems
- Meeus, J. (1998): Astronomical Algorithms
- Altitude-Azimuth coordinate system transformations

---

## License

See [LICENSE](LICENSE) file for details.

---

## Changelog

### Version 1.0.0 (January 10, 2026)

**Features**:
- Initial release
- Database-driven architecture
- AirplanesLive API integration
- PostgreSQL schema with delta tracking
- NASR waypoint database (72,313 waypoints, 997 airways)
- FlightAware API integration (Phase 2)
- Waypoint-based prediction (Phase 3)
- Airway-based prediction (Phase 4)
- TUI viewfinder with all enhancements
- Comprehensive configuration system
- Docker deployment

**Tools**:
- collector: Background data collection service
- track-aircraft-db: Database-driven tracking with prediction
- tui-viewfinder: Interactive TUI with zoom, trails, vectors
- import-nasr: NASR data import
- fetch-flightplans: FlightAware integration
- verification tools

**Documentation**:
- Complete wiki
- Configuration guide
- Database architecture
- NASR import guide
- FlightAware integration guide
- Airway prediction documentation

---

*Last Updated: January 10, 2026*  
*Wiki Version: 1.0.0*  
*Project Version: 1.0.0*
