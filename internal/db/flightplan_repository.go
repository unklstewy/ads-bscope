package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FlightPlanRepository handles database operations for flight plans and routes.
type FlightPlanRepository struct {
	db *DB
}

// NewFlightPlanRepository creates a new flight plan repository.
func NewFlightPlanRepository(db *DB) *FlightPlanRepository {
	return &FlightPlanRepository{db: db}
}

// FlightPlan represents a filed flight plan in the database.
type FlightPlan struct {
	ID            int
	ICAO          string
	Callsign      string
	DepartureICAO string
	ArrivalICAO   string
	Route         string // Full route string (e.g., "KCLT..CHSLY.J121.ATL..KATL")
	FiledAltitude int    // Feet MSL
	AircraftType  string
	FiledTime     time.Time
	ETD           time.Time
	ETA           time.Time
	LastUpdated   time.Time
}

// FlightPlanRoute represents a resolved waypoint in a flight plan.
type FlightPlanRoute struct {
	ID           int
	FlightPlanID int
	Sequence     int
	WaypointID   int
	WaypointName string
	Latitude     float64
	Longitude    float64
	ETA          *time.Time
	Passed       bool
}

// Waypoint represents a navigation waypoint from the NASR database.
type Waypoint struct {
	ID         int
	Identifier string
	Name       string
	Latitude   float64
	Longitude  float64
	Type       string // fix, vor, ndb, tacan, airport
	Region     string
}

// UpsertFlightPlan inserts or updates a flight plan.
//
// If a flight plan already exists for the same ICAO, it updates it.
// This is useful when periodically refreshing flight plan data.
func (r *FlightPlanRepository) UpsertFlightPlan(ctx context.Context, fp FlightPlan) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO flight_plans (
			icao, callsign, departure_icao, arrival_icao, route,
			filed_altitude, aircraft_type, filed_time, etd, eta, last_updated
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (icao) DO UPDATE SET
			callsign = EXCLUDED.callsign,
			departure_icao = EXCLUDED.departure_icao,
			arrival_icao = EXCLUDED.arrival_icao,
			route = EXCLUDED.route,
			filed_altitude = EXCLUDED.filed_altitude,
			aircraft_type = EXCLUDED.aircraft_type,
			filed_time = EXCLUDED.filed_time,
			etd = EXCLUDED.etd,
			eta = EXCLUDED.eta,
			last_updated = EXCLUDED.last_updated
		RETURNING id`,
		fp.ICAO, fp.Callsign, fp.DepartureICAO, fp.ArrivalICAO, fp.Route,
		fp.FiledAltitude, fp.AircraftType, fp.FiledTime, fp.ETD, fp.ETA, fp.LastUpdated,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to upsert flight plan: %w", err)
	}

	return id, nil
}

// GetFlightPlanByICAO retrieves a flight plan by aircraft ICAO code.
func (r *FlightPlanRepository) GetFlightPlanByICAO(ctx context.Context, icao string) (*FlightPlan, error) {
	var fp FlightPlan
	err := r.db.QueryRowContext(ctx,
		`SELECT id, icao, callsign, departure_icao, arrival_icao, route,
		        filed_altitude, aircraft_type, filed_time, etd, eta, last_updated
		 FROM flight_plans
		 WHERE icao = $1`,
		icao,
	).Scan(
		&fp.ID, &fp.ICAO, &fp.Callsign, &fp.DepartureICAO, &fp.ArrivalICAO, &fp.Route,
		&fp.FiledAltitude, &fp.AircraftType, &fp.FiledTime, &fp.ETD, &fp.ETA, &fp.LastUpdated,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get flight plan: %w", err)
	}

	return &fp, nil
}

// GetWaypointByIdentifier looks up a waypoint by its identifier (e.g., "CHSLY", "ATL").
//
// If multiple waypoints exist with the same identifier (e.g., different regions),
// this returns the first match. For more precise matching, use GetWaypointsByIdentifier.
func (r *FlightPlanRepository) GetWaypointByIdentifier(ctx context.Context, identifier string) (*Waypoint, error) {
	var wp Waypoint
	err := r.db.QueryRowContext(ctx,
		`SELECT id, identifier, COALESCE(name, ''), latitude, longitude, type, COALESCE(region, '')
		 FROM waypoints
		 WHERE identifier = $1
		 LIMIT 1`,
		identifier,
	).Scan(&wp.ID, &wp.Identifier, &wp.Name, &wp.Latitude, &wp.Longitude, &wp.Type, &wp.Region)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get waypoint: %w", err)
	}

	return &wp, nil
}

// GetWaypointsByIdentifier returns all waypoints matching an identifier.
//
// Some waypoints exist in multiple regions or with different types.
// Use this when you need to select the most appropriate match.
func (r *FlightPlanRepository) GetWaypointsByIdentifier(ctx context.Context, identifier string) ([]Waypoint, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, identifier, COALESCE(name, ''), latitude, longitude, type, COALESCE(region, '')
		 FROM waypoints
		 WHERE identifier = $1
		 ORDER BY type, region`,
		identifier,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query waypoints: %w", err)
	}
	defer rows.Close()

	var waypoints []Waypoint
	for rows.Next() {
		var wp Waypoint
		if err := rows.Scan(&wp.ID, &wp.Identifier, &wp.Name, &wp.Latitude, &wp.Longitude, &wp.Type, &wp.Region); err != nil {
			return nil, fmt.Errorf("failed to scan waypoint: %w", err)
		}
		waypoints = append(waypoints, wp)
	}

	return waypoints, rows.Err()
}

// FindAirportsNear finds airports within a given radius of a position.
// This is useful for selecting airports to center radar views on.
//
// Parameters:
//   - lat, lon: Center position
//   - radiusNM: Search radius in nautical miles
//   - limit: Maximum number of airports to return (0 = no limit)
//
// Returns: List of airports sorted by distance (nearest first)
func (r *FlightPlanRepository) FindAirportsNear(
	ctx context.Context,
	lat, lon float64,
	radiusNM float64,
	limit int,
) ([]Waypoint, error) {
	// Convert radius to approximate lat/lon delta
	// 1 degree latitude ≈ 60 NM
	latDelta := radiusNM / 60.0
	lonDelta := radiusNM / 60.0

	// Build query
	query := `
		SELECT id, identifier, COALESCE(name, ''), latitude, longitude, type, COALESCE(region, '')
		FROM waypoints
		WHERE type = 'airport'
		  AND latitude BETWEEN $1 - $3 AND $1 + $3
		  AND longitude BETWEEN $2 - $4 AND $2 + $4
		ORDER BY 
			-- Sort by approximate distance (Manhattan distance)
			ABS(latitude - $1) + ABS(longitude - $2)
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.QueryContext(ctx, query, lat, lon, latDelta, lonDelta)
	if err != nil {
		return nil, fmt.Errorf("failed to query airports: %w", err)
	}
	defer rows.Close()

	var airports []Waypoint
	for rows.Next() {
		var wp Waypoint
		if err := rows.Scan(&wp.ID, &wp.Identifier, &wp.Name, &wp.Latitude, &wp.Longitude, &wp.Type, &wp.Region); err != nil {
			return nil, fmt.Errorf("failed to scan airport: %w", err)
		}
		airports = append(airports, wp)
	}

	return airports, rows.Err()
}

// ParseAndStoreRoute parses a route string and stores the waypoint sequence.
//
// Route format examples:
// - "KCLT..CHSLY.J121.ATL..KATL" (airport, direct, fix, airway, vor, direct, airport)
// - "KCLT.CHSLY.J121.ATL.KATL" (with dots between all waypoints)
// - "KCLT CHSLY J121 ATL KATL" (space-separated)
//
// The parser handles:
// - Direct waypoints (simple identifiers)
// - Airways (preceded by airway identifier like J121, V1)
// - DCT (explicit direct routing)
// - .. (implied direct routing)
//
// Returns the number of waypoints resolved, or error if route cannot be parsed.
func (r *FlightPlanRepository) ParseAndStoreRoute(ctx context.Context, flightPlanID int, routeString string) (int, error) {
	if routeString == "" {
		return 0, nil
	}

	// Clean up route string
	routeString = strings.TrimSpace(routeString)

	// Replace ".." with single space for easier parsing
	routeString = strings.ReplaceAll(routeString, "..", " ")

	// Replace single dots with spaces
	routeString = strings.ReplaceAll(routeString, ".", " ")

	// Split into tokens
	tokens := strings.Fields(routeString)

	// Delete existing route waypoints for this flight plan
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM flight_plan_routes WHERE flight_plan_id = $1`,
		flightPlanID,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete existing route: %w", err)
	}

	sequence := 0
	resolvedCount := 0

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]

		// Skip "DCT" (direct routing indicator)
		if token == "DCT" {
			continue
		}

		// Check if this is an airway identifier
		if isAirway(token) {
			// Airway waypoints will come after, just skip the identifier
			// In the future, we can resolve airway segments here
			continue
		}

		// Try to resolve as waypoint
		wp, err := r.GetWaypointByIdentifier(ctx, token)
		if err != nil {
			return resolvedCount, fmt.Errorf("failed to lookup waypoint %s: %w", token, err)
		}

		if wp == nil {
			// Waypoint not found in database - log but continue
			// This can happen with airways or special routing instructions
			continue
		}

		// Insert waypoint into route sequence
		_, err = r.db.ExecContext(ctx,
			`INSERT INTO flight_plan_routes (
				flight_plan_id, sequence, waypoint_id, passed
			) VALUES ($1, $2, $3, FALSE)`,
			flightPlanID, sequence, wp.ID,
		)
		if err != nil {
			return resolvedCount, fmt.Errorf("failed to insert route waypoint: %w", err)
		}

		sequence++
		resolvedCount++
	}

	return resolvedCount, nil
}

// isAirway checks if a token represents an airway identifier.
//
// Airways typically start with:
// - J (Jet route, high altitude)
// - V (Victor airway, low altitude)
// - Q/T (RNAV routes)
// - A/B/G/R (other route types)
func isAirway(token string) bool {
	if len(token) < 2 {
		return false
	}

	first := token[0]
	return (first == 'J' || first == 'V' || first == 'Q' ||
		first == 'T' || first == 'A' || first == 'B' ||
		first == 'G' || first == 'R') &&
		len(token) <= 5 // Airways are typically 2-5 chars
}

// GetFlightPlanRoute retrieves the resolved waypoint sequence for a flight plan.
//
// Returns waypoints in order (by sequence number), including coordinates for each.
func (r *FlightPlanRepository) GetFlightPlanRoute(ctx context.Context, flightPlanID int) ([]FlightPlanRoute, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT fpr.id, fpr.flight_plan_id, fpr.sequence, fpr.waypoint_id, 
		        w.identifier, w.latitude, w.longitude, fpr.eta, fpr.passed
		 FROM flight_plan_routes fpr
		 JOIN waypoints w ON w.id = fpr.waypoint_id
		 WHERE fpr.flight_plan_id = $1
		 ORDER BY fpr.sequence ASC`,
		flightPlanID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query route: %w", err)
	}
	defer rows.Close()

	var routes []FlightPlanRoute
	for rows.Next() {
		var route FlightPlanRoute
		var eta sql.NullTime

		if err := rows.Scan(
			&route.ID, &route.FlightPlanID, &route.Sequence, &route.WaypointID,
			&route.WaypointName, &route.Latitude, &route.Longitude, &eta, &route.Passed,
		); err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}

		if eta.Valid {
			route.ETA = &eta.Time
		}

		routes = append(routes, route)
	}

	return routes, rows.Err()
}

// MarkWaypointPassed marks a waypoint in a flight plan route as passed.
//
// This is useful for tracking progress along a route and determining
// which waypoint the aircraft is currently heading towards.
func (r *FlightPlanRepository) MarkWaypointPassed(ctx context.Context, flightPlanID, sequence int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE flight_plan_routes 
		 SET passed = TRUE
		 WHERE flight_plan_id = $1 AND sequence = $2`,
		flightPlanID, sequence,
	)
	return err
}

// GetNextWaypoint returns the next waypoint that hasn't been passed yet.
//
// This is used by the prediction algorithm to determine where the aircraft
// is likely heading.
func (r *FlightPlanRepository) GetNextWaypoint(ctx context.Context, flightPlanID int) (*FlightPlanRoute, error) {
	var route FlightPlanRoute
	var eta sql.NullTime

	err := r.db.QueryRowContext(ctx,
		`SELECT fpr.id, fpr.flight_plan_id, fpr.sequence, fpr.waypoint_id,
		        w.identifier, w.latitude, w.longitude, fpr.eta, fpr.passed
		 FROM flight_plan_routes fpr
		 JOIN waypoints w ON w.id = fpr.waypoint_id
		 WHERE fpr.flight_plan_id = $1 AND fpr.passed = FALSE
		 ORDER BY fpr.sequence ASC
		 LIMIT 1`,
		flightPlanID,
	).Scan(
		&route.ID, &route.FlightPlanID, &route.Sequence, &route.WaypointID,
		&route.WaypointName, &route.Latitude, &route.Longitude, &eta, &route.Passed,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get next waypoint: %w", err)
	}

	if eta.Valid {
		route.ETA = &eta.Time
	}

	return &route, nil
}

// AirwaySegment represents a segment of an airway between two waypoints.
type AirwaySegment struct {
	AirwayID     string
	AirwayType   string // victor, jet, rnav
	Sequence     int
	FromWaypoint Waypoint
	ToWaypoint   Waypoint
	MinAltitude  int
	MaxAltitude  int
	Bearing      float64 // Bearing from FromWaypoint to ToWaypoint
	DistanceNM   float64 // Distance in nautical miles
}

// FindNearbyAirways finds airways within a given radius of a position.
// This is used to match aircraft to airways when no flight plan is available.
//
// Parameters:
//   - lat, lon: Aircraft position
//   - radiusNM: Search radius in nautical miles (recommend 10-25 NM)
//   - minAltitude, maxAltitude: Filter by altitude (0 = no filter)
//
// Returns: List of airway segments within radius
func (r *FlightPlanRepository) FindNearbyAirways(
	ctx context.Context,
	lat, lon float64,
	radiusNM float64,
	minAltitude, maxAltitude int,
) ([]AirwaySegment, error) {
	// Convert radius to approximate lat/lon delta
	// 1 degree latitude ≈ 60 NM
	// 1 degree longitude varies by latitude, but use ~60 NM as approximation
	latDelta := radiusNM / 60.0
	lonDelta := radiusNM / 60.0

	query := `
		SELECT 
			a1.identifier,
			a1.type,
			a1.sequence,
			w1.id, w1.identifier, COALESCE(w1.name, ''), w1.latitude, w1.longitude, w1.type, COALESCE(w1.region, ''),
			w2.id, w2.identifier, COALESCE(w2.name, ''), w2.latitude, w2.longitude, w2.type, COALESCE(w2.region, ''),
			COALESCE(a1.min_altitude, 0),
			COALESCE(a1.max_altitude, 99999)
		FROM airways a1
		JOIN waypoints w1 ON a1.waypoint_id = w1.id
		JOIN airways a2 ON a1.identifier = a2.identifier 
		                  AND a1.type = a2.type 
		                  AND a2.sequence = a1.sequence + 1
		JOIN waypoints w2 ON a2.waypoint_id = w2.id
		WHERE 
			-- Check if either waypoint is within search box
			(w1.latitude BETWEEN $1 - $3 AND $1 + $3
			 AND w1.longitude BETWEEN $2 - $4 AND $2 + $4)
			OR
			(w2.latitude BETWEEN $1 - $3 AND $1 + $3
			 AND w2.longitude BETWEEN $2 - $4 AND $2 + $4)
	`

	// Add altitude filtering if specified
	if minAltitude > 0 || maxAltitude > 0 {
		if maxAltitude == 0 {
			maxAltitude = 99999
		}
		query += `
			AND (COALESCE(a1.max_altitude, 99999) >= $5
			     AND COALESCE(a1.min_altitude, 0) <= $6)
		`
	}

	query += ` ORDER BY a1.identifier, a1.sequence`

	var rows *sql.Rows
	var err error

	if minAltitude > 0 || maxAltitude > 0 {
		if maxAltitude == 0 {
			maxAltitude = 99999
		}
		rows, err = r.db.QueryContext(ctx, query, lat, lon, latDelta, lonDelta, minAltitude, maxAltitude)
	} else {
		rows, err = r.db.QueryContext(ctx, query, lat, lon, latDelta, lonDelta)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query airways: %w", err)
	}
	defer rows.Close()

	var segments []AirwaySegment
	for rows.Next() {
		var seg AirwaySegment
		var w1, w2 Waypoint

		err := rows.Scan(
			&seg.AirwayID, &seg.AirwayType, &seg.Sequence,
			&w1.ID, &w1.Identifier, &w1.Name, &w1.Latitude, &w1.Longitude, &w1.Type, &w1.Region,
			&w2.ID, &w2.Identifier, &w2.Name, &w2.Latitude, &w2.Longitude, &w2.Type, &w2.Region,
			&seg.MinAltitude, &seg.MaxAltitude,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan airway: %w", err)
		}

		seg.FromWaypoint = w1
		seg.ToWaypoint = w2

		segments = append(segments, seg)
	}

	return segments, rows.Err()
}
