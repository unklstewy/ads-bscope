-- ADS-B Scope Database Schema
-- PostgreSQL schema for storing aircraft positions and tracking history

-- Aircraft table: stores current state of each tracked aircraft
CREATE TABLE IF NOT EXISTS aircraft (
    -- Identity
    icao TEXT PRIMARY KEY,                    -- ICAO 24-bit address (e.g., "a12345")
    callsign TEXT,                            -- Flight callsign/registration
    
    -- Current position (WGS84)
    latitude DOUBLE PRECISION NOT NULL,       -- Decimal degrees (-90 to +90)
    longitude DOUBLE PRECISION NOT NULL,      -- Decimal degrees (-180 to +180)
    altitude_ft DOUBLE PRECISION,             -- Altitude in feet MSL
    
    -- Current velocity
    ground_speed_kts DOUBLE PRECISION,        -- Ground speed in knots
    track_deg DOUBLE PRECISION,               -- Ground track in degrees (0-360)
    vertical_rate_fpm DOUBLE PRECISION,       -- Vertical rate in feet/minute
    
    -- Tracking metadata
    first_seen TIMESTAMP NOT NULL,            -- First time seen in current session
    last_seen TIMESTAMP NOT NULL,             -- Most recent position update
    last_updated TIMESTAMP NOT NULL,          -- Last time this record was updated
    position_count INTEGER DEFAULT 1,         -- Number of position reports received
    
    -- Computed values (updated on each position update)
    range_nm DOUBLE PRECISION,                -- Distance from observer in nautical miles
    bearing_deg DOUBLE PRECISION,             -- Bearing from observer in degrees
    altitude_deg DOUBLE PRECISION,            -- Altitude angle from observer (elevation)
    azimuth_deg DOUBLE PRECISION,             -- Azimuth from observer (0=N, 90=E)
    
    -- Approach tracking
    is_approaching BOOLEAN DEFAULT FALSE,     -- Currently approaching observer
    closest_range_nm DOUBLE PRECISION,        -- Predicted closest approach distance
    eta_closest_seconds INTEGER,              -- ETA to closest approach
    
    -- Status flags
    is_visible BOOLEAN DEFAULT TRUE,          -- Currently within tracking range
    is_trackable BOOLEAN DEFAULT FALSE,       -- Within telescope altitude limits
    last_trackable TIMESTAMP,                 -- Last time aircraft was trackable
    
    -- Indexes for performance
    CONSTRAINT valid_latitude CHECK (latitude BETWEEN -90 AND 90),
    CONSTRAINT valid_longitude CHECK (longitude BETWEEN -180 AND 180),
    CONSTRAINT valid_altitude CHECK (altitude_ft IS NULL OR altitude_ft >= -1000)
);

-- Aircraft position history: stores time-series position data
CREATE TABLE IF NOT EXISTS aircraft_positions (
    id BIGSERIAL PRIMARY KEY,
    icao TEXT NOT NULL REFERENCES aircraft(icao) ON DELETE CASCADE,
    
    -- Position snapshot
    timestamp TIMESTAMP NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    altitude_ft DOUBLE PRECISION,
    
    -- Velocity snapshot
    ground_speed_kts DOUBLE PRECISION,
    track_deg DOUBLE PRECISION,
    vertical_rate_fpm DOUBLE PRECISION,
    
    -- Calculated deltas (from previous position)
    delta_time_seconds DOUBLE PRECISION,      -- Time since previous position
    delta_distance_nm DOUBLE PRECISION,       -- Distance traveled
    delta_altitude_ft DOUBLE PRECISION,       -- Altitude change
    delta_track_deg DOUBLE PRECISION,         -- Track change (handles wrap-around)
    
    -- Derived velocities (calculated from actual deltas)
    actual_speed_kts DOUBLE PRECISION,        -- Speed calculated from position delta
    actual_vertical_rate_fpm DOUBLE PRECISION, -- V/S calculated from altitude delta
    
    -- Observer-relative measurements
    range_nm DOUBLE PRECISION,
    bearing_deg DOUBLE PRECISION,
    altitude_angle_deg DOUBLE PRECISION,
    azimuth_deg DOUBLE PRECISION
);

-- Session metadata: tracks data collection sessions
CREATE TABLE IF NOT EXISTS tracking_sessions (
    id SERIAL PRIMARY KEY,
    start_time TIMESTAMP NOT NULL DEFAULT NOW(),
    end_time TIMESTAMP,
    observer_latitude DOUBLE PRECISION NOT NULL,
    observer_longitude DOUBLE PRECISION NOT NULL,
    observer_elevation_m DOUBLE PRECISION NOT NULL,
    search_radius_nm DOUBLE PRECISION NOT NULL,
    update_interval_seconds INTEGER NOT NULL,
    total_updates INTEGER DEFAULT 0,
    unique_aircraft INTEGER DEFAULT 0,
    notes TEXT
);

-- Observer configuration: stores observer location history
CREATE TABLE IF NOT EXISTS observer_locations (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    elevation_m DOUBLE PRECISION NOT NULL,
    timezone TEXT NOT NULL,
    description TEXT,
    is_current BOOLEAN DEFAULT TRUE
);

-- Telescope tracking log: records when telescope was commanded to track aircraft
CREATE TABLE IF NOT EXISTS telescope_tracking_log (
    id BIGSERIAL PRIMARY KEY,
    icao TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Aircraft state at tracking time
    aircraft_latitude DOUBLE PRECISION NOT NULL,
    aircraft_longitude DOUBLE PRECISION NOT NULL,
    aircraft_altitude_ft DOUBLE PRECISION,
    aircraft_range_nm DOUBLE PRECISION,
    
    -- Telescope command
    telescope_altitude_deg DOUBLE PRECISION NOT NULL,
    telescope_azimuth_deg DOUBLE PRECISION NOT NULL,
    mount_type TEXT NOT NULL,                -- 'altaz' or 'equatorial'
    
    -- Tracking result
    command_sent BOOLEAN DEFAULT FALSE,
    command_success BOOLEAN,
    error_message TEXT,
    
    -- Prediction info
    predicted_position BOOLEAN DEFAULT FALSE,
    prediction_latency_seconds DOUBLE PRECISION,
    prediction_confidence DOUBLE PRECISION
);

-- Indexes for performance

-- Aircraft lookups
CREATE INDEX IF NOT EXISTS idx_aircraft_last_seen ON aircraft(last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_aircraft_is_visible ON aircraft(is_visible) WHERE is_visible = TRUE;
CREATE INDEX IF NOT EXISTS idx_aircraft_is_trackable ON aircraft(is_trackable) WHERE is_trackable = TRUE;
CREATE INDEX IF NOT EXISTS idx_aircraft_approaching ON aircraft(is_approaching) WHERE is_approaching = TRUE;
CREATE INDEX IF NOT EXISTS idx_aircraft_range ON aircraft(range_nm);

-- Position history lookups
CREATE INDEX IF NOT EXISTS idx_positions_icao_timestamp ON aircraft_positions(icao, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_positions_timestamp ON aircraft_positions(timestamp DESC);

-- Tracking log lookups
CREATE INDEX IF NOT EXISTS idx_tracking_log_timestamp ON telescope_tracking_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_tracking_log_icao ON telescope_tracking_log(icao, timestamp DESC);

-- Waypoints table: navigation fixes, VORs, NDBs, and airports
CREATE TABLE IF NOT EXISTS waypoints (
    id SERIAL PRIMARY KEY,
    identifier TEXT NOT NULL,  -- e.g., "CHSLY", "ATL"
    name TEXT,                 -- Full name
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    type TEXT NOT NULL,        -- fix, vor, ndb, airport, intersection
    region TEXT,               -- e.g., "K1" for US East
    description TEXT,
    UNIQUE(identifier, region)
);

-- Airways table: Victor airways, Jet routes, RNAV routes
CREATE TABLE IF NOT EXISTS airways (
    id SERIAL PRIMARY KEY,
    identifier TEXT NOT NULL,  -- e.g., "J121", "V1"
    type TEXT NOT NULL,        -- victor, jet, rnav, other
    sequence INTEGER NOT NULL, -- Order of waypoints along airway
    waypoint_id INTEGER REFERENCES waypoints(id) ON DELETE CASCADE,
    min_altitude INTEGER,      -- Minimum altitude in feet
    max_altitude INTEGER,      -- Maximum altitude in feet
    direction TEXT,            -- forward, backward, bidirectional
    UNIQUE(identifier, sequence)
);

-- Flight plans table: filed or retrieved flight plans
CREATE TABLE IF NOT EXISTS flight_plans (
    id SERIAL PRIMARY KEY,
    icao TEXT NOT NULL,
    callsign TEXT NOT NULL,
    departure_icao TEXT,       -- Departure airport code
    arrival_icao TEXT,         -- Arrival airport code
    route TEXT,                -- Full route string
    filed_altitude INTEGER,    -- Filed cruise altitude in feet
    aircraft_type TEXT,        -- Aircraft type code
    filed_time TIMESTAMP,
    etd TIMESTAMP,             -- Estimated time of departure
    eta TIMESTAMP,             -- Estimated time of arrival
    last_updated TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_flight_plan_aircraft FOREIGN KEY (icao) REFERENCES aircraft(icao) ON DELETE CASCADE
);

-- Flight plan routes: resolved waypoints from flight plans
CREATE TABLE IF NOT EXISTS flight_plan_routes (
    id SERIAL PRIMARY KEY,
    flight_plan_id INTEGER NOT NULL REFERENCES flight_plans(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    waypoint_id INTEGER NOT NULL REFERENCES waypoints(id),
    eta TIMESTAMP,             -- Estimated time at waypoint
    passed BOOLEAN DEFAULT FALSE,
    UNIQUE(flight_plan_id, sequence)
);

-- Indexes for waypoints and airways
CREATE INDEX IF NOT EXISTS idx_waypoints_identifier ON waypoints(identifier);
CREATE INDEX IF NOT EXISTS idx_waypoints_type ON waypoints(type);
CREATE INDEX IF NOT EXISTS idx_waypoints_location ON waypoints(latitude, longitude);

CREATE INDEX IF NOT EXISTS idx_airways_identifier ON airways(identifier);
CREATE INDEX IF NOT EXISTS idx_airways_type ON airways(type);
CREATE INDEX IF NOT EXISTS idx_airways_sequence ON airways(identifier, sequence);

CREATE INDEX IF NOT EXISTS idx_flight_plans_icao ON flight_plans(icao);
CREATE INDEX IF NOT EXISTS idx_flight_plans_callsign ON flight_plans(callsign);

CREATE INDEX IF NOT EXISTS idx_flight_plan_routes_plan ON flight_plan_routes(flight_plan_id, sequence);

-- Comments for documentation
COMMENT ON TABLE aircraft IS 'Current state of all tracked aircraft within range';
COMMENT ON TABLE aircraft_positions IS 'Time-series history of aircraft positions for velocity/acceleration analysis';
COMMENT ON TABLE tracking_sessions IS 'Metadata about data collection sessions';
COMMENT ON TABLE telescope_tracking_log IS 'Log of telescope tracking commands sent to aircraft';
COMMENT ON TABLE waypoints IS 'Navigation waypoints, fixes, VORs, NDBs, and airports from FAA NASR';
COMMENT ON TABLE airways IS 'Victor airways, Jet routes, and RNAV routes with waypoint sequences';
COMMENT ON TABLE flight_plans IS 'Filed flight plans retrieved from external APIs';
COMMENT ON TABLE flight_plan_routes IS 'Resolved waypoint sequences from flight plans';

COMMENT ON COLUMN aircraft.position_count IS 'Number of position updates received - used for data quality assessment';
COMMENT ON COLUMN aircraft.is_trackable IS 'Whether aircraft is within telescope altitude limits (considering imaging mode)';
COMMENT ON COLUMN aircraft_positions.actual_speed_kts IS 'Ground speed calculated from actual position delta - more accurate than reported speed';
