# FlightAware Integration

This document describes the FlightAware AeroAPI v4 integration for enhanced flight plan prediction.

## Overview

The FlightAware integration retrieves filed flight plans for tracked aircraft, enabling waypoint-based prediction instead of simple dead reckoning. When aircraft leave ADS-B coverage, the system can predict their position along their filed route.

## Setup

### 1. Get an API Key

Sign up for FlightAware AeroAPI at: https://www.flightaware.com/aeroapi/

**Tiers:**
- **Free**: 500 requests/month (~0.7 requests/hour)
- **Basic**: 250,000 requests/month (~340 requests/hour) - $49.99/month

### 2. Configure

Add your API key to `configs/config.json`:

```json
{
  "flightaware": {
    "api_key": "YOUR_API_KEY_HERE",
    "enabled": true,
    "requests_per_hour": 10,
    "auto_fetch_enabled": true,
    "fetch_interval_minutes": 60
  }
}
```

Or set via environment variable:
```bash
export ADS_BSCOPE_FLIGHTAWARE_API_KEY="YOUR_API_KEY_HERE"
```

### 3. Configuration Options

- **`api_key`**: Your FlightAware API key
- **`enabled`**: Enable/disable FlightAware integration
- **`requests_per_hour`**: Rate limit for API calls (default: 10)
  - Free tier: Use 1 to stay within 500/month
  - Basic tier: Can use up to 340
- **`auto_fetch_enabled`**: Automatically fetch plans for tracked aircraft
- **`fetch_interval_minutes`**: How often to refresh plans (default: 60)

## Usage

### Verify API Connection

Test the API with a callsign:

```bash
go run cmd/verify-flightplans/main.go UAL123
```

Output:
```
✓ Flight Plan Retrieved:
  Callsign: UAL123
  Route: KDFW → PHOG
  Aircraft: B789
  Altitude: 39000 ft
  Route String: KDFW..TURKI.J56.MLU..PHOG
  Status: Active
```

### Fetch Flight Plans

Run the fetcher service to retrieve plans for active aircraft:

```bash
go run cmd/fetch-flightplans/main.go
```

The fetcher will:
1. Query database for aircraft seen in last 5 minutes with callsigns
2. Fetch flight plans from FlightAware
3. Parse route strings and resolve waypoints
4. Store in database for prediction use

**Output:**
```
===========================================
  FlightAware Flight Plan Fetcher
===========================================
API Rate Limit: 10 requests/hour
Fetch Interval: 60 minutes
===========================================

Found 3 active aircraft with callsigns
  → Fetching flight plan for UAL123 (ABCD12)...
    ✓ Stored: KDFW → PHOG (8 waypoints)
  → Fetching flight plan for DAL456 (EFGH34)...
    - No flight plan found
  ✓ AAL789 (IJKL56) - Using cached flight plan

===========================================
Fetch Summary:
  Success: 1
  Not Found: 1
  Errors: 0
===========================================
```

### Check Database Status

View stored flight plans:

```bash
go run cmd/verify-flightplans/main.go
```

Output:
```
Total Flight Plans: 5

Recent Flight Plans:
  UAL123: KDFW → PHOG (B789, 39000 ft, 8 waypoints)
  DAL456: KATL → KLAX (B738, 35000 ft, 12 waypoints)
  AAL789: KJFK → EGLL (A359, 41000 ft, 6 waypoints)

Waypoint Resolution:
  Plans with resolved waypoints: 3/5 (60.0%)
```

## Architecture

### Components

1. **FlightAware Client** (`pkg/flightaware/client.go`)
   - HTTP client for AeroAPI v4
   - Rate limiting to respect API quotas
   - Handles authentication and error responses

2. **Flight Plan Repository** (`internal/db/flightplan_repository.go`)
   - Database operations for flight plans and routes
   - Route string parser (handles formats like "KCLT..CHSLY.J121.ATL..KATL")
   - Waypoint resolution against NASR database

3. **Fetcher Service** (`cmd/fetch-flightplans/main.go`)
   - Periodic background service
   - Fetches plans for active aircraft
   - Caches results to avoid redundant API calls

### Database Schema

**flight_plans** table:
```sql
CREATE TABLE flight_plans (
    id SERIAL PRIMARY KEY,
    icao TEXT NOT NULL,
    callsign TEXT NOT NULL,
    departure_icao TEXT NOT NULL,
    arrival_icao TEXT NOT NULL,
    route TEXT,
    filed_altitude INTEGER,
    aircraft_type TEXT,
    filed_time TIMESTAMP,
    etd TIMESTAMP,
    eta TIMESTAMP,
    last_updated TIMESTAMP NOT NULL
);
```

**flight_plan_routes** table:
```sql
CREATE TABLE flight_plan_routes (
    id SERIAL PRIMARY KEY,
    flight_plan_id INTEGER REFERENCES flight_plans(id),
    sequence INTEGER NOT NULL,
    waypoint_id INTEGER REFERENCES waypoints(id),
    eta TIMESTAMP,
    passed BOOLEAN DEFAULT FALSE
);
```

## Route String Parsing

The parser handles various route string formats:

- **"KCLT..CHSLY.J121.ATL..KATL"** - Airport, direct, fix, airway, VOR, direct, airport
- **"KCLT.CHSLY.J121.ATL.KATL"** - Single dots between all waypoints
- **"KCLT CHSLY J121 ATL KATL"** - Space-separated
- **"KCLT DCT CHSLY J121 ATL DCT KATL"** - Explicit direct routing

**Parsing Logic:**
1. Clean up separators (convert ".." and "." to spaces)
2. Split into tokens
3. Skip "DCT" (direct routing indicators)
4. Identify airways (J123, V1, etc.) and skip identifiers
5. Resolve remaining tokens as waypoints in NASR database
6. Store waypoint sequence with coordinates

## API Limits and Best Practices

### Rate Limiting

The client automatically enforces rate limits:
```go
faClient := flightaware.NewClient(flightaware.Config{
    APIKey:          "your-key",
    RequestsPerHour: 10,  // Enforced by rate limiter
})
```

### Caching

Flight plans are cached in the database for 1 hour to avoid redundant API calls:
```go
if existing != nil && time.Since(existing.LastUpdated) < time.Hour {
    // Use cached plan
}
```

### Recommended Settings

**Free Tier (500/month):**
- `requests_per_hour: 1`
- `fetch_interval_minutes: 120` (refresh every 2 hours)
- Estimated usage: ~360 requests/month

**Basic Tier (250,000/month):**
- `requests_per_hour: 50`
- `fetch_interval_minutes: 15` (refresh every 15 minutes)
- Estimated usage: ~2,400 requests/month for 5 aircraft

## Troubleshooting

### No Route Strings

Some flight plans may not include route strings:
- VFR flights typically don't file routes
- Some international flights have restricted route data
- FlightAware's free tier may have limited route access

The system will still store departure/arrival airports for these flights.

### API Errors

**401 Unauthorized:**
- Check API key in config
- Verify key is valid at https://www.flightaware.com/aeroapi/portal/

**429 Too Many Requests:**
- Reduce `requests_per_hour` in config
- Increase `fetch_interval_minutes`

**404 Not Found:**
- Aircraft has no filed flight plan (common for VFR, military, private)
- Callsign may be incorrect

### Waypoint Resolution Failures

If waypoints aren't resolving:
1. Verify NASR data is imported: `go run cmd/verify-nasr/main.go`
2. Check for typos in route string
3. Some waypoints may be region-specific or not in NASR database

## Next Steps (Phase 3)

Once flight plans are being fetched and stored:

1. **Integrate with Prediction Algorithm** - Use waypoint coordinates for position prediction
2. **Waypoint Progress Tracking** - Mark waypoints as "passed" based on aircraft position
3. **Next Waypoint Prediction** - Predict position along great circle to next waypoint
4. **Confidence Scoring** - Higher confidence when following known route

See `docs/NASR_IMPORT.md` for waypoint database details.
