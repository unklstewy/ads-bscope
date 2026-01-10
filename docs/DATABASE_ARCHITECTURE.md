# Database-Driven Architecture

## Overview

The ADS-B Scope application uses a database-driven architecture to efficiently collect and share aircraft tracking data. This approach solves several key problems:

1. **API Rate Limiting**: Single collector service respects API limits; multiple clients query the database
2. **Historical Data**: Store position history for accurate velocity/acceleration calculations
3. **Shared State**: Multiple users/trackers share the same real-time data
4. **Improved Predictions**: Use actual position deltas instead of reported velocities

## Architecture Components

### 1. Collector Service (`cmd/collector`)

**Purpose**: Background service that continuously fetches aircraft data from ADS-B sources and stores it in PostgreSQL.

**Features**:
- Respects API rate limits (configurable per source)
- Calculates observer-relative measurements (range, bearing, altitude, azimuth)
- Computes position deltas (distance, time, altitude, track)
- Calculates actual velocities from position changes (more accurate than reported)
- Marks aircraft as trackable based on telescope limits
- Auto-cleanup of stale data
- Real-time statistics

**Usage**:
```bash
# Start the collector service
go run cmd/collector/main.go --config configs/config.json

# Or build and run
go build -o bin/collector cmd/collector/main.go
./bin/collector --config configs/config.json
```

### 2. Database Schema

**Tables**:
- `aircraft`: Current state of all tracked aircraft
- `aircraft_positions`: Time-series history with calculated deltas
- `tracking_sessions`: Session metadata
- `telescope_tracking_log`: Telescope command history
- `observer_locations`: Observer location history

**Key Features**:
- Automatic delta calculations (time, distance, altitude, track changes)
- Actual velocity calculation from position deltas
- Observer-relative measurements pre-calculated
- Approach detection and ETA tracking
- Trackable status based on telescope limits

### 3. Aircraft Repository (`internal/db/aircraft_repository.go`)

**Purpose**: Data access layer for aircraft operations.

**Key Methods**:
- `UpsertAircraft()`: Insert/update aircraft with automatic delta calculation
- `GetTrackableAircraft()`: Get all aircraft within telescope limits
- `GetAircraftByICAO()`: Retrieve specific aircraft
- `GetPositionHistory()`: Get historical positions for prediction
- `UpdateTrackableStatus()`: Update trackability based on altitude limits
- `CalculateAverageVelocity()`: Compute average velocity from history

## Database Schema

### Aircraft Table
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
    is_trackable BOOLEAN DEFAULT FALSE
);
```

### Aircraft Positions Table
```sql
CREATE TABLE aircraft_positions (
    id BIGSERIAL PRIMARY KEY,
    icao TEXT NOT NULL REFERENCES aircraft(icao),
    timestamp TIMESTAMP NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude_ft DOUBLE PRECISION,
    
    -- Calculated deltas (from previous position)
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

## Benefits of Delta Tracking

### 1. Accurate Velocity Calculation
- **Reported speed**: From aircraft transponder (may have errors/delays)
- **Actual speed**: Calculated from position changes over time
- **Result**: More reliable predictions

### 2. Trend Detection
- Track acceleration/deceleration
- Detect turns (track changes)
- Monitor climb/descent rates
- Identify unusual flight patterns

### 3. Improved Prediction
- Use historical velocity trends
- Account for acceleration
- Better ETA calculations
- Smoother tracking

## Configuration

Update `configs/config.json`:

```json
{
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
}
```

## Docker Setup

The collector service runs alongside PostgreSQL in Docker:

```bash
# Start database and collector
docker-compose up -d

# View collector logs
docker-compose logs -f ads-bscope

# Stop services
docker-compose down
```

## Performance Characteristics

**Collector Service**:
- ~1 API call per update interval (e.g., every 10 seconds)
- Stores 100+ aircraft per update
- ~10-20ms per aircraft upsert
- Automatic cleanup every 5 minutes

**Query Performance**:
- Aircraft lookups: <1ms
- Trackable aircraft query: <5ms
- Position history (last 5 min): <10ms
- Statistics query: <5ms

**Storage**:
- ~1KB per aircraft record
- ~500 bytes per position record
- ~5MB per hour (100 aircraft, 10s intervals)
- Automatic cleanup keeps last 24 hours

## Future Enhancements

1. **Machine Learning**:
   - Predict flight paths from historical patterns
   - Detect anomalies
   - Optimize tracking strategies

2. **Multi-Observer**:
   - Support multiple observer locations
   - Triangulation for improved accuracy
   - Collaborative tracking

3. **Analytics**:
   - Flight pattern analysis
   - Traffic density heatmaps
   - Best tracking windows

4. **Caching**:
   - Redis for hot data
   - Reduce database load
   - Faster queries

5. **API**:
   - REST/WebSocket API for web clients
   - Real-time updates
   - Multi-user support
